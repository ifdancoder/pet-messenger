// Package message is the usecase layer for messages: sending (with optional
// per-conversation idempotency), editing and deleting one's own messages, and
// paging a conversation's history. It orchestrates the message + conversation
// domain ports; it does not re-implement domain rules.
package message

import (
	"context"
	"errors"
	"fmt"
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
)

// Service is the message usecase.
type Service struct {
	messages      domainmsg.Repository
	conversations domainconv.Repository
}

// NewService wires the service. messages is implemented by
// internal/infrastructure/postgres.MessageRepository; conversations by
// internal/infrastructure/postgres.ConversationRepository.
func NewService(messages domainmsg.Repository, conversations domainconv.Repository) *Service {
	return &Service{messages: messages, conversations: conversations}
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
	member, err := s.conversations.GetMember(ctx, convID, actor)
	if err != nil {
		return nil, err // conversation.ErrNotMember
	}
	if !conv.CanPost(member.Role) {
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
	return m, nil
}

// DeleteMessage soft-deletes actor's own message in convID (idempotent). It
// returns the tombstoned message (Content cleared, DeletedAt set).
func (s *Service) DeleteMessage(ctx context.Context, actor, convID, messageID uuid.UUID) (*domainmsg.Message, error) {
	if _, err := s.conversations.GetMember(ctx, convID, actor); err != nil {
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
		return nil, ErrNotAuthor
	}
	m.MarkDeleted(time.Now().UTC())
	if err := s.messages.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	return m, nil
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
