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
)

// ErrContentTooLong is reported when trimmed content exceeds maxContentRunes. It
// is built from the same constant so the limit is stated in exactly one place.
var ErrContentTooLong = fmt.Errorf("message: content must be at most %d runes", maxContentRunes)

// Message is a single message in a conversation. SenderID is nil for system and
// call_event messages. ReplyToID, EditedAt, and DeletedAt are nil until set.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
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
	Create(ctx context.Context, m *Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*Message, error)
	// ListByConversation returns messages ordered by (CreatedAt, ID) ascending.
	// When after is non-nil it is a message id used as a keyset cursor: the
	// implementation resolves it to that message's (CreatedAt, ID) pair and
	// returns only rows strictly greater than that pair — the tuple, not
	// CreatedAt alone, because IDs are random UUIDv4 and CreatedAt is not
	// unique. This is the reconnect catch-up from spec §4. Soft-deleted
	// messages ARE returned, with Content cleared. limit caps the page size;
	// limit <= 0 is a caller error.
	ListByConversation(ctx context.Context, conversationID uuid.UUID, after *uuid.UUID, limit int) ([]*Message, error)
	// Update persists a mutated message (Edit / MarkDeleted).
	Update(ctx context.Context, m *Message) error
}
