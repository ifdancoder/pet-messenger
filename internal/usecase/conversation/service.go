// Package conversation is the usecase layer for the conversation aggregate:
// opening direct messages, creating group chats, and listing a user's
// conversations. It orchestrates the domain ports; it does not re-implement
// domain rules.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/user"
)

// Page-size bounds for ListMine.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

var (
	// ErrRecipientNotFound: OpenDirect was asked to start a DM with a user id
	// that does not resolve to an account.
	ErrRecipientNotFound = errors.New("conversation: recipient user does not exist")
	// ErrCannotDirectSelf: OpenDirect was called with me == other.
	ErrCannotDirectSelf = errors.New("conversation: cannot open a direct conversation with yourself")
)

// Service is the conversation usecase.
type Service struct {
	conversations domainconv.Repository
	users         user.Repository
}

// NewService wires the service. conversations is implemented by
// internal/infrastructure/postgres.ConversationRepository; users by
// internal/infrastructure/postgres.UserRepository.
func NewService(conversations domainconv.Repository, users user.Repository) *Service {
	return &Service{conversations: conversations, users: users}
}

// EffectiveListLimit clamps a client-requested page size: non-positive means
// "use the default"; anything above the max is capped.
func EffectiveListLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultListLimit
	case requested > MaxListLimit:
		return MaxListLimit
	default:
		return requested
	}
}

// OpenDirect returns the direct conversation between me and other, creating it
// if it does not exist yet. It is idempotent: concurrent first-opens converge on
// one row (the unique direct_key makes the loser's insert fail with
// ErrDirectExists, which we resolve by re-reading).
func (s *Service) OpenDirect(ctx context.Context, me, other uuid.UUID) (*domainconv.Conversation, error) {
	if me == other {
		return nil, ErrCannotDirectSelf
	}
	if _, err := s.users.GetByID(ctx, other); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrRecipientNotFound
		}
		return nil, fmt.Errorf("load recipient: %w", err)
	}

	if existing, err := s.conversations.GetDirect(ctx, me, other); err == nil {
		return existing, nil
	} else if !errors.Is(err, domainconv.ErrNotFound) {
		return nil, fmt.Errorf("lookup direct: %w", err)
	}

	c, members, err := domainconv.NewDirect(me, other)
	if err != nil {
		return nil, err // ErrInvalidDirectMembers
	}
	if err := s.conversations.Create(ctx, c, members); err != nil {
		if errors.Is(err, domainconv.ErrDirectExists) {
			raced, err := s.conversations.GetDirect(ctx, me, other)
			if err != nil {
				return nil, fmt.Errorf("reload direct after create race: %w", err)
			}
			return raced, nil
		}
		return nil, fmt.Errorf("create direct: %w", err)
	}
	return c, nil
}

// CreateGroupChat creates a group chat owned by creator. Title validation
// (trim, 1..256) lives in the domain constructor.
func (s *Service) CreateGroupChat(ctx context.Context, creator uuid.UUID, title string) (*domainconv.Conversation, error) {
	c, members, err := domainconv.NewGroupChat(creator, title)
	if err != nil {
		return nil, err // ErrInvalidCreator / ErrInvalidTitle
	}
	if err := s.conversations.Create(ctx, c, members); err != nil {
		return nil, fmt.Errorf("create group chat: %w", err)
	}
	return c, nil
}

// ListMine returns the caller's conversations newest-activity-first. A zero
// `before` starts from the newest; otherwise the (before, beforeID) pair is the
// keyset cursor of the last row already seen. limit is clamped via
// EffectiveListLimit.
//
// The keyset sorts on last_message_at, a value that changes under the reader.
// A conversation currently below the client's cursor that receives a new
// message jumps above the cursor and is then skipped by every later page, so a
// straight walk to the end can miss it. This is inherent to keyset pagination
// over an activity timestamp; the client mitigation is to re-fetch from the top
// whenever new activity arrives rather than only paging downward.
func (s *Service) ListMine(ctx context.Context, me uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	list, err := s.conversations.ListForUser(ctx, me, before, beforeID, EffectiveListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return list, nil
}

// CreateChannel creates a channel_group or channel_broadcast owned by creator.
// Kind/title validation lives in the domain constructor.
func (s *Service) CreateChannel(ctx context.Context, creator uuid.UUID, kind domainconv.Kind, title string, public bool) (*domainconv.Conversation, error) {
	c, members, err := domainconv.NewChannel(creator, kind, title, public)
	if err != nil {
		return nil, err // ErrInvalidCreator / ErrInvalidKind / ErrInvalidTitle
	}
	if err := s.conversations.Create(ctx, c, members); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return c, nil
}

// JoinChannel adds actor to a public channel and returns the channel. It is
// idempotent: an actor who is already a member gets the channel back unchanged.
// A non-channel target, or a channel that is not public, is refused as
// ErrNotFound, so neither a private channel's nor a non-channel conversation's
// existence is disclosed.
func (s *Service) JoinChannel(ctx context.Context, actor, channelID uuid.UUID) (*domainconv.Conversation, error) {
	conv, err := s.conversations.GetByID(ctx, channelID)
	if err != nil {
		return nil, err // conversation.ErrNotFound
	}
	if !conv.Kind.IsChannel() {
		return nil, domainconv.ErrNotFound
	}
	if !conv.Public {
		return nil, domainconv.ErrNotFound
	}
	if _, err := s.conversations.GetMember(ctx, channelID, actor); err == nil {
		return conv, nil
	} else if !errors.Is(err, domainconv.ErrNotMember) {
		return nil, fmt.Errorf("check membership: %w", err)
	}

	role := domainconv.RoleMember
	if conv.Kind == domainconv.KindChannelBroadcast {
		role = domainconv.RoleSubscriber
	}
	m := &domainconv.Member{
		ConversationID: channelID,
		UserID:         actor,
		Role:           role,
		JoinedAt:       time.Now().UTC(),
	}
	if err := s.conversations.AddMember(ctx, m); err != nil {
		return nil, fmt.Errorf("add channel member: %w", err)
	}
	return conv, nil
}

// LeaveChannel removes actor's membership from a channel. A non-channel target
// is refused as ErrNotFound, so a non-channel conversation's existence is not
// disclosed. The owner may leave; the channel then has no owner until ownership
// is transferred (a later slice).
//
// NOTE: this removes the membership row but does not tear down the departed
// user's live WebSocket subscription — they keep receiving this conversation's
// real-time events on an already-open socket until they unsubscribe or
// reconnect. A cross-pod membership-revocation event for the Hub is deferred to
// the moderation slice.
//
// The owner may leave (a deliberate product choice); for a channel_broadcast
// this leaves the channel with no one who may post (CanPost requires
// owner/admin) while it stays public and discoverable. Ownership transfer,
// admin promotion, and channel delete/archive are a prerequisite slice before
// launch.
func (s *Service) LeaveChannel(ctx context.Context, actor, channelID uuid.UUID) error {
	conv, err := s.conversations.GetByID(ctx, channelID)
	if err != nil {
		return err // conversation.ErrNotFound
	}
	if !conv.Kind.IsChannel() {
		return domainconv.ErrNotFound
	}
	return s.conversations.RemoveMember(ctx, channelID, actor) // conversation.ErrNotMember
}

// ListPublicChannels returns discoverable channels newest-activity-first. query
// is a case-insensitive title substring; limit is clamped via EffectiveListLimit.
func (s *Service) ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	list, err := s.conversations.ListPublicChannels(ctx, strings.TrimSpace(query), before, beforeID, EffectiveListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list public channels: %w", err)
	}
	return list, nil
}
