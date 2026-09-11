// Package message is the usecase layer for messages: sending (with optional
// per-conversation idempotency), editing and deleting one's own messages, and
// paging a conversation's history. It orchestrates the message + conversation
// domain ports; it does not re-implement domain rules.
package message

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
)

// Page-size bounds for ListMessages.
const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

var (
	// ErrCannotPost: the caller is a member but their role may not post here
	// (e.g. a subscriber in a broadcast channel).
	ErrCannotPost = errors.New("message: role may not post to this conversation")
	// ErrNotAuthor: edit/delete attempted by someone other than the sender.
	ErrNotAuthor = errors.New("message: only the author may edit or delete this message")
	// ErrReplyNotFound: the reply_to target does not exist or is in another
	// conversation.
	ErrReplyNotFound = errors.New("message: reply target does not exist in this conversation")
	// ErrConflictingCursors: ListMessages was given both afterSeq and beforeSeq.
	ErrConflictingCursors = errors.New("message: specify at most one of after_seq / before_seq")
	// ErrConversationArchived: SendMessage was attempted against an archived
	// conversation.
	ErrConversationArchived = errors.New("message: conversation is archived")
)

// Service is the message usecase.
type Service struct {
	messages      domainmsg.Repository
	conversations domainconv.Repository
	publisher     MessagePublisher
	notif         NotificationProducer
}

// NewService wires the service. messages is implemented by
// internal/infrastructure/postgres.MessageRepository; conversations by
// internal/infrastructure/postgres.ConversationRepository; publisher by
// internal/infrastructure/nats.Publisher; notif by
// internal/infrastructure/notification.Producer.
func NewService(messages domainmsg.Repository, conversations domainconv.Repository, publisher MessagePublisher, notif NotificationProducer) *Service {
	return &Service{messages: messages, conversations: conversations, publisher: publisher, notif: notif}
}

// PageLimit clamps a client-requested page size: non-positive means "use the
// default"; anything above the max is capped.
func PageLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultPageLimit
	case requested > MaxPageLimit:
		return MaxPageLimit
	default:
		return requested
	}
}

// SendMessage posts a text message from actor to convID. idempotencyKey is
// optional ("" = none): a retry with the same key returns the original message
// instead of creating a duplicate. replyToID, when set, must reference a message
// in the same conversation.
func (s *Service) SendMessage(ctx context.Context, actor, convID uuid.UUID, content string, replyToID *uuid.UUID, idempotencyKey string) (*domainmsg.Message, error) {
	conv, err := s.conversations.GetByID(ctx, convID)
	if err != nil {
		return nil, err // conversation.ErrNotFound
	}
	if conv.IsArchived() {
		return nil, ErrConversationArchived
	}
	member, err := s.conversations.GetMember(ctx, convID, actor)
	if err != nil {
		return nil, err // conversation.ErrNotMember
	}
	if !conv.CanPost(member) {
		return nil, ErrCannotPost
	}
	if replyToID != nil {
		target, err := s.messages.GetByID(ctx, *replyToID)
		if err != nil {
			if errors.Is(err, domainmsg.ErrNotFound) {
				return nil, ErrReplyNotFound
			}
			return nil, fmt.Errorf("load reply target: %w", err)
		}
		if target.ConversationID != convID {
			return nil, ErrReplyNotFound
		}
	}

	m, err := domainmsg.NewText(convID, actor, content, replyToID)
	if err != nil {
		return nil, err // ErrEmptyContent / ErrContentTooLong / ...
	}
	if idempotencyKey != "" {
		m.IdempotencyKey = &idempotencyKey
	}

	if err := s.messages.Create(ctx, m); err != nil {
		if errors.Is(err, domainmsg.ErrDuplicateIdempotencyKey) {
			existing, err := s.messages.GetByIdempotencyKey(ctx, convID, actor, idempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("reload after idempotency race: %v", err)
			}
			return existing, nil
		}
		return nil, fmt.Errorf("create message: %w", err)
	}
	if err := s.publisher.PublishCreated(ctx, m); err != nil {
		log.Printf("message: publish created %s: %v", m.ID, err)
	}
	if err := s.notif.MessageSent(ctx, m, conv); err != nil {
		log.Printf("message: notify message-sent %s: %v", m.ID, err)
	}
	return m, nil
}

// EditMessage replaces the content of actor's own text message in convID. The
// message must belong to convID and actor must be both a current member and the
// original sender.
func (s *Service) EditMessage(ctx context.Context, actor, convID, messageID uuid.UUID, newContent string) (*domainmsg.Message, error) {
	if _, err := s.conversations.GetMember(ctx, convID, actor); err != nil {
		return nil, err // conversation.ErrNotMember
	}
	m, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return nil, err // message.ErrNotFound
	}
	if m.ConversationID != convID {
		return nil, domainmsg.ErrNotFound
	}
	if m.SenderID == nil || *m.SenderID != actor {
		return nil, ErrNotAuthor
	}
	if err := m.Edit(newContent, time.Now().UTC()); err != nil {
		return nil, err // message.ErrNotEditable / ErrEmptyContent / ErrContentTooLong
	}
	if err := s.messages.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	if err := s.publisher.PublishUpdated(ctx, m); err != nil {
		log.Printf("message: publish updated %s: %v", m.ID, err)
	}
	return m, nil
}

// DeleteMessage soft-deletes actor's own message in convID (idempotent). It
// returns the tombstoned message (Content cleared, DeletedAt set).
func (s *Service) DeleteMessage(ctx context.Context, actor, convID, messageID uuid.UUID) (*domainmsg.Message, error) {
	member, err := s.conversations.GetMember(ctx, convID, actor)
	if err != nil {
		return nil, err
	}
	m, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if m.ConversationID != convID {
		return nil, domainmsg.ErrNotFound
	}
	if m.SenderID == nil || *m.SenderID != actor {
		if !member.CanModerateMessages() {
			return nil, ErrNotAuthor
		}
	}
	m.MarkDeleted(time.Now().UTC())
	if err := s.messages.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	if err := s.publisher.PublishDeleted(ctx, m); err != nil {
		log.Printf("message: publish deleted %s: %v", m.ID, err)
	}
	return m, nil
}

// PostSystemEvent records a membership / lifecycle change as a system message
// in conv's timeline, so it gets a gap-free seq and fans out live as
// message.created. It is a no-op (returns nil) for direct conversations, and
// for channel_broadcast when the event is a member_* event (join/leave/kick
// churn there would flood the timeline). conversation_archived /
// conversation_unarchived are posted for every non-direct kind; call_started /
// call_ended are posted for every kind, direct included. A publish failure is
// logged; a persist failure is returned.
func (s *Service) PostSystemEvent(ctx context.Context, conv *domainconv.Conversation, actorID *uuid.UUID, event domainmsg.SystemEvent, targetID uuid.UUID, detail string) error {
	isCallEvent := event == domainmsg.SystemCallStarted || event == domainmsg.SystemCallEnded
	if conv.Kind == domainconv.KindDirect && !isCallEvent {
		return nil
	}
	isConversationEvent := event == domainmsg.SystemConversationArchived || event == domainmsg.SystemConversationUnarchived
	if !isConversationEvent && !isCallEvent && conv.Kind == domainconv.KindChannelBroadcast {
		return nil
	}
	m, err := domainmsg.NewSystemEvent(conv.ID, actorID, event, targetID, detail)
	if err != nil {
		return err
	}
	if err := s.messages.Create(ctx, m); err != nil {
		return fmt.Errorf("create system event: %w", err)
	}
	if err := s.publisher.PublishCreated(ctx, m); err != nil {
		log.Printf("message: publish system event %s: %v", m.ID, err)
	}
	switch event {
	case domainmsg.SystemMemberRemoved:
		if err := s.notif.MembershipEvent(ctx, event, conv.ID, targetID, ""); err != nil {
			log.Printf("message: notify membership-event %s: %v", conv.ID, err)
		}
	case domainmsg.SystemMemberTierChanged:
		if err := s.notif.MembershipEvent(ctx, event, conv.ID, targetID, detail); err != nil {
			log.Printf("message: notify membership-event %s: %v", conv.ID, err)
		}
	}
	return nil
}

// ListMessages returns a page of convID's history for a member. At most one of
// afterSeq / beforeSeq may be set: afterSeq walks forward (ascending, the
// reconnect catch-up read), beforeSeq walks backward (descending, history
// scroll). Both nil means "newest page" (descending from the end). limit is
// clamped via PageLimit.
func (s *Service) ListMessages(ctx context.Context, actor, convID uuid.UUID, afterSeq, beforeSeq *int64, limit int) ([]*domainmsg.Message, error) {
	if _, err := s.conversations.GetMember(ctx, convID, actor); err != nil {
		return nil, err // conversation.ErrNotMember
	}
	if afterSeq != nil && beforeSeq != nil {
		return nil, ErrConflictingCursors
	}
	n := PageLimit(limit)
	if afterSeq != nil {
		return s.messages.ListByConversation(ctx, convID, *afterSeq, n)
	}
	before := int64(0)
	if beforeSeq != nil {
		before = *beforeSeq
	}
	return s.messages.ListBeforeSeq(ctx, convID, before, n)
}
