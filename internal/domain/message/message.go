// Package message holds the Message aggregate root: chat/channel messages of
// several kinds, keyed by Type. Message owns its own state transitions (Edit,
// MarkDeleted); the usecase layer orchestrates, it does not reimplement them.
package message

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Type discriminates message kinds. text and system are usable now; attachment
// and call_event are defined for schema completeness and get their constructors
// once the media/storage stages land.
type Type string

// Message types.
const (
	TypeText       Type = "text"
	TypeAttachment Type = "attachment"
	TypeSystem     Type = "system"
	TypeCallEvent  Type = "call_event"
)

// Valid reports whether t is one of the four known types.
func (t Type) Valid() bool {
	switch t {
	case TypeText, TypeAttachment, TypeSystem, TypeCallEvent:
		return true
	default:
		return false
	}
}

// maxContentRunes bounds message content length, measured in runes.
const maxContentRunes = 4096

// Domain errors.
var (
	ErrEmptyContent        = errors.New("message: content must not be empty")
	ErrInvalidSender       = errors.New("message: sender must be a valid user id")
	ErrInvalidConversation = errors.New("message: conversation must be a valid conversation id")
	ErrNotEditable         = errors.New("message: only a non-deleted text message can be edited")
	ErrNotFound            = errors.New("message: not found")
	// ErrInvalidLimit is returned by Repository.ListByConversation for limit <= 0.
	ErrInvalidLimit = errors.New("message: limit must be a positive integer")
	// ErrConversationNotFound is returned by Repository.Create when the target
	// conversation does not exist.
	ErrConversationNotFound = errors.New("message: conversation does not exist")
)

// ErrContentTooLong is reported when trimmed content exceeds maxContentRunes. It
// is built from the same constant so the limit is stated in exactly one place.
var ErrContentTooLong = fmt.Errorf("message: content must be at most %d runes", maxContentRunes)

// Message is a single message in a conversation. Seq is the per-conversation
// monotonic sequence number assigned by Repository.Create (0 until persisted);
// it is the ordering and pagination key. The sequence is contiguous only while
// messages are never hard-deleted (today they are only soft-deleted via
// MarkDeleted); a future hard delete would leave a permanent gap. SenderID is
// nil for system and call_event messages. ReplyToID, EditedAt, and DeletedAt are
// nil until set. A text message read back from storage may also have a nil
// SenderID if the author's account was later deleted (sender_id is ON DELETE SET
// NULL); consumers rendering a text message must handle that.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Seq            int64
	SenderID       *uuid.UUID
	Type           Type
	Content        string
	ReplyToID      *uuid.UUID
	EditedAt       *time.Time
	DeletedAt      *time.Time
	CreatedAt      time.Time
}

// validateContent trims surrounding whitespace and enforces the 1..4096-rune
// bound. It returns the trimmed content.
func validateContent(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	switch {
	case c == "":
		return "", ErrEmptyContent
	case utf8.RuneCountInString(c) > maxContentRunes:
		return "", ErrContentTooLong
	default:
		return c, nil
	}
}

// NewText builds a text message from senderID in conversationID. content is
// trimmed and must be 1..4096 runes. replyToID is optional and passed through
// as-is (existence is a usecase/repository concern).
func NewText(conversationID, senderID uuid.UUID, content string, replyToID *uuid.UUID) (*Message, error) {
	if conversationID == uuid.Nil {
		return nil, ErrInvalidConversation
	}
	if senderID == uuid.Nil {
		return nil, ErrInvalidSender
	}
	c, err := validateContent(content)
	if err != nil {
		return nil, err
	}
	if replyToID != nil && *replyToID == uuid.Nil {
		replyToID = nil
	}
	sid := senderID
	return &Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: conversationID,
		SenderID:       &sid,
		Type:           TypeText,
		Content:        c,
		ReplyToID:      replyToID,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// NewSystem builds a system message (no sender) in conversationID, e.g.
// "alice joined the group". content is trimmed and must be 1..4096 runes.
func NewSystem(conversationID uuid.UUID, content string) (*Message, error) {
	if conversationID == uuid.Nil {
		return nil, ErrInvalidConversation
	}
	c, err := validateContent(content)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: conversationID,
		Type:           TypeSystem,
		Content:        c,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// IsEdited reports whether the message has been edited.
func (m *Message) IsEdited() bool { return m.EditedAt != nil }

// IsDeleted reports whether the message has been soft-deleted.
func (m *Message) IsDeleted() bool { return m.DeletedAt != nil }

// Edit replaces the content of a non-deleted text message and records the edit
// time. It returns ErrNotEditable for any non-text or deleted message, and the
// usual content errors if newContent is empty or too long. A rejected edit
// leaves the message unchanged.
func (m *Message) Edit(newContent string, now time.Time) error {
	if m.Type != TypeText || m.IsDeleted() {
		return ErrNotEditable
	}
	c, err := validateContent(newContent)
	if err != nil {
		return err
	}
	m.Content = c
	utc := now.UTC()
	m.EditedAt = &utc
	return nil
}

// MarkDeleted soft-deletes the message: it records the deletion time (normalized
// to UTC) and clears the stored content so deleted text is not retained. It is
// idempotent — calling it on an already-deleted message is a no-op that keeps
// the first deletion time.
func (m *Message) MarkDeleted(now time.Time) {
	if m.IsDeleted() {
		return
	}
	utc := now.UTC()
	m.DeletedAt = &utc
	m.Content = ""
}

// Repository is the persistence port for messages. It is implemented in
// internal/infrastructure/postgres in a later stage. The method set grows as
// usecases arrive.
type Repository interface {
	// Create persists the message, allocating and setting m.Seq (the
	// per-conversation monotonic sequence) as part of the write.
	Create(ctx context.Context, m *Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*Message, error)
	// ListByConversation returns the conversation's messages ordered by Seq
	// ascending, starting after afterSeq (afterSeq <= 0 means from the first
	// message). Seq is a per-conversation monotonic sequence the Repository
	// allocates at commit time under a row lock, so the result is gap-free and
	// in commit order: a client that passes the highest Seq of the contiguous
	// prefix it holds is guaranteed not to miss a message (the spec §4 catch-up
	// path). Because seq is gap-free, a client that receives a Seq more than one
	// above its last contiguous Seq knows it has a gap and must re-fetch from the
	// contiguous watermark. Soft-deleted messages are returned, with Content
	// cleared. A limit <= 0 returns ErrInvalidLimit.
	ListByConversation(ctx context.Context, conversationID uuid.UUID, afterSeq int64, limit int) ([]*Message, error)
	// Update persists a mutated message (Edit / MarkDeleted).
	Update(ctx context.Context, m *Message) error
}
