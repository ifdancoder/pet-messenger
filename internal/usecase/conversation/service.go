// Package conversation is the usecase layer for the conversation aggregate:
// opening direct messages, creating group chats, and listing a user's
// conversations. It orchestrates the domain ports; it does not re-implement
// domain rules.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
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
	// ErrForbidden: the actor is a member but their tier does not permit this
	// action on this target.
	ErrForbidden = errors.New("conversation: actor is not permitted to perform this action")
	// ErrCannotKickSelf: RemoveMember was called with actor == target; use
	// LeaveConversation instead.
	ErrCannotKickSelf = errors.New("conversation: cannot remove yourself; leave instead")
	// ErrInvalidTier: SetMemberTier was given an unknown admin_tier value.
	ErrInvalidTier = errors.New("conversation: unknown admin tier")
	// ErrInviteeNotFound: AddMember was asked to add a user id that does not
	// resolve to an account.
	ErrInviteeNotFound = errors.New("conversation: invitee user does not exist")
)

// Service is the conversation usecase.
type Service struct {
	conversations domainconv.Repository
	users         user.Repository
	revoker       MemberEventPublisher
	systemEvents  SystemEventPoster
}

// NewService wires the service. conversations is implemented by
// internal/infrastructure/postgres.ConversationRepository; users by
// internal/infrastructure/postgres.UserRepository; revoker by
// internal/infrastructure/nats.Publisher; systemEvents by
// *internal/usecase/message.Service.
func NewService(conversations domainconv.Repository, users user.Repository, revoker MemberEventPublisher, systemEvents SystemEventPoster) *Service {
	return &Service{conversations: conversations, users: users, revoker: revoker, systemEvents: systemEvents}
}

// postEvent records a membership / lifecycle change on the conversation
// timeline. It is best-effort: the mutation has already committed, so a failed
// post is logged and the caller still returns success.
func (s *Service) postEvent(ctx context.Context, conv *domainconv.Conversation, actor *uuid.UUID, event domainmsg.SystemEvent, target uuid.UUID, detail string) {
	if err := s.systemEvents.PostSystemEvent(ctx, conv, actor, event, target, detail); err != nil {
		log.Printf("conversation: post system event %s on %s: %v", event, conv.ID, err)
	}
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
// existence is disclosed; only once the target is known to be a public channel
// is an archived one reported as ErrConversationArchived.
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
	if conv.IsArchived() {
		return nil, domainconv.ErrConversationArchived
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
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberJoined, actor, "")
	return conv, nil
}

// LeaveConversation removes actor's own membership from a group chat or channel
// and publishes a revoke so every pod drops their live subscription. A direct
// conversation cannot be left (refused as ErrNotFound). The creator may leave;
// the conversation then has no creator (a known gap — see the spec).
//
// The publish is best-effort: the membership row is already gone, so a failed
// revoke is logged and the call still succeeds.
func (s *Service) LeaveConversation(ctx context.Context, actor, conversationID uuid.UUID) error {
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return err // conversation.ErrNotFound
	}
	if conv.Kind == domainconv.KindDirect {
		return domainconv.ErrNotFound
	}
	promoted, err := s.conversations.RemoveMemberWithSuccession(ctx, conversationID, actor)
	if err != nil {
		return err // conversation.ErrNotMember
	}
	if err := s.revoker.PublishMemberRevoked(ctx, conversationID, actor); err != nil {
		log.Printf("conversation: publish member revoked %s/%s: %v", conversationID, actor, err)
	}
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberLeft, actor, "")
	if promoted != nil {
		s.postEvent(ctx, conv, nil, domainmsg.SystemMemberTierChanged, promoted.UserID, string(domainconv.TierCreatorAdmin))
	}
	return nil
}

// AddMember adds invitee to a group chat or channel on actor's authority. actor
// must hold an admin tier (rank >= 2) in a non-direct conversation. A
// channel_broadcast invitee joins as a subscriber; everywhere else as a plain
// member.
func (s *Service) AddMember(ctx context.Context, actor, conversationID, invitee uuid.UUID) (*domainconv.Member, error) {
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err // conversation.ErrNotFound
	}
	if conv.IsArchived() {
		return nil, domainconv.ErrConversationArchived
	}
	actorM, err := s.conversations.GetMember(ctx, conversationID, actor)
	if err != nil {
		return nil, err // conversation.ErrNotMember
	}
	if !conv.CanAddMembers(actorM) {
		return nil, ErrForbidden
	}
	if _, err := s.users.GetByID(ctx, invitee); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInviteeNotFound
		}
		return nil, fmt.Errorf("load invitee: %w", err)
	}
	role := domainconv.RoleMember
	if conv.Kind == domainconv.KindChannelBroadcast {
		role = domainconv.RoleSubscriber
	}
	m := &domainconv.Member{
		ConversationID: conversationID,
		UserID:         invitee,
		Role:           role,
		AdminTier:      domainconv.TierNone,
		JoinedAt:       time.Now().UTC(),
	}
	if err := s.conversations.AddMember(ctx, m); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}
	added, err := s.conversations.GetMember(ctx, conversationID, invitee)
	if err != nil {
		return nil, fmt.Errorf("reload added member: %w", err)
	}
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberJoined, invitee, "")
	return added, nil
}

// RemoveMember kicks target on actor's authority and publishes a revoke. Use
// LeaveConversation to remove yourself. Rank rules live in the domain
// (CanManageMember).
func (s *Service) RemoveMember(ctx context.Context, actor, conversationID, target uuid.UUID) error {
	if actor == target {
		return ErrCannotKickSelf
	}
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return err // conversation.ErrNotFound
	}
	if conv.Kind == domainconv.KindDirect {
		return domainconv.ErrNotFound // a DM participant is never kickable, regardless of tier
	}
	actorM, err := s.conversations.GetMember(ctx, conversationID, actor)
	if err != nil {
		return err // conversation.ErrNotMember
	}
	targetM, err := s.conversations.GetMember(ctx, conversationID, target)
	if err != nil {
		return err // conversation.ErrNotMember -> target absent
	}
	if !domainconv.CanManageMember(actorM, targetM) {
		return ErrForbidden
	}
	if err := s.conversations.RemoveMember(ctx, conversationID, target); err != nil {
		return err
	}
	if err := s.revoker.PublishMemberRevoked(ctx, conversationID, target); err != nil {
		log.Printf("conversation: publish member revoked %s/%s: %v", conversationID, target, err)
	}
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberRemoved, target, "")
	return nil
}

// SetMemberTier promotes, demotes, or retiers target on actor's authority. It
// does not publish a revoke — a retiered member stays in the conversation. Who
// may assign what lives in the domain (CanAssignTier).
func (s *Service) SetMemberTier(ctx context.Context, actor, conversationID, target uuid.UUID, tier domainconv.AdminTier) (*domainconv.Member, error) {
	if !tier.Valid() {
		return nil, ErrInvalidTier
	}
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err // conversation.ErrNotFound
	}
	actorM, err := s.conversations.GetMember(ctx, conversationID, actor)
	if err != nil {
		return nil, err // conversation.ErrNotMember
	}
	targetM, err := s.conversations.GetMember(ctx, conversationID, target)
	if err != nil {
		return nil, err // conversation.ErrNotMember
	}
	if !domainconv.CanAssignTier(actorM, targetM, tier) {
		return nil, ErrForbidden
	}
	if err := s.conversations.SetMemberTier(ctx, conversationID, target, tier); err != nil {
		return nil, err
	}
	updated, err := s.conversations.GetMember(ctx, conversationID, target)
	if err != nil {
		return nil, fmt.Errorf("reload retiered member: %w", err)
	}
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberTierChanged, target, string(tier))
	return updated, nil
}

// SetArchived archives or unarchives a conversation on actor's authority
// (superadmin+). Archiving makes it read-only and hides it from lists and
// discovery; unarchiving restores it.
func (s *Service) SetArchived(ctx context.Context, actor, conversationID uuid.UUID, archived bool) error {
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return err // conversation.ErrNotFound
	}
	if conv.IsArchived() == archived {
		return nil // already in the requested state: no repo write, no timeline entry
	}
	actorM, err := s.conversations.GetMember(ctx, conversationID, actor)
	if err != nil {
		return err // conversation.ErrNotMember
	}
	if !domainconv.CanArchive(actorM) {
		return ErrForbidden
	}
	if err := s.conversations.SetArchived(ctx, conversationID, archived); err != nil {
		return err
	}
	event := domainmsg.SystemConversationArchived
	if !archived {
		event = domainmsg.SystemConversationUnarchived
	}
	s.postEvent(ctx, conv, &actor, event, uuid.Nil, "")
	return nil
}

// ClaimCreator makes actor the creator of a conversation whose seat is vacant.
// A direct conversation is refused as ErrNotFound (it is seat-less by
// construction). A non-member is refused as ErrNotMember before any creator
// state is inspected. Only the highest-ranked current member may claim
// (ErrNotCreatorHeir); ErrCreatorSeatTaken if a creator already exists.
func (s *Service) ClaimCreator(ctx context.Context, actor, conversationID uuid.UUID) (*domainconv.Member, error) {
	conv, err := s.conversations.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err // conversation.ErrNotFound
	}
	if conv.Kind == domainconv.KindDirect {
		return nil, domainconv.ErrNotFound // DMs are seat-less by construction and never claimable
	}
	if _, err := s.conversations.GetMember(ctx, conversationID, actor); err != nil {
		return nil, err // conversation.ErrNotMember — a non-member gets 404, not a creator-state oracle
	}
	m, err := s.conversations.ClaimCreator(ctx, conversationID, actor)
	if err != nil {
		return nil, err // ErrCreatorSeatTaken / ErrNotCreatorHeir / ErrNotMember
	}
	s.postEvent(ctx, conv, &actor, domainmsg.SystemMemberTierChanged, actor, string(domainconv.TierCreatorAdmin))
	return m, nil
}

// ListMembers returns a page of the conversation's roster. Any current member
// may read it. adminsOnly restricts to tier-holders and the creator.
func (s *Service) ListMembers(ctx context.Context, actor, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.MemberProfile, error) {
	if _, err := s.conversations.GetByID(ctx, conversationID); err != nil {
		return nil, err // conversation.ErrNotFound
	}
	if _, err := s.conversations.GetMember(ctx, conversationID, actor); err != nil {
		return nil, err // conversation.ErrNotMember
	}
	list, err := s.conversations.ListMembers(ctx, conversationID, adminsOnly, before, beforeUserID, EffectiveListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return list, nil
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
