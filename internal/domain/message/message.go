// Package message holds the Message aggregate root: chat/channel messages of
// several kinds, keyed by Type. Message owns its own state transitions (Edit,
// MarkDeleted); the usecase layer orchestrates, it does not reimplement them.
package message

import (
	"context"
	"errors"
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

// Domain errors.
var (
	ErrEmptyContent   = errors.New("message: content must not be empty")
	ErrContentTooLong = errors.New("message: content must be at most 4096 characters")
	ErrInvalidSender  = errors.New("message: sender must be a valid user id")
	ErrNotEditable    = errors.New("message: only a non-deleted text message can be edited")
	ErrNotFound       = errors.New("message: not found")
)

const maxContentRunes = 4096

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
	if senderID == uuid.Nil {
		return nil, ErrInvalidSender
	}
	c, err := validateContent(content)
	if err != nil {
		return nil, err
	}
	sid := senderID
	return &Message{
		ID:             uuid.New(),
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
	c, err := validateContent(content)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:             uuid.New(),
		ConversationID: conversationID,
		Type:           TypeSystem,
		Content:        c,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// Repository is the persistence port for messages. It is implemented in
// internal/infrastructure/postgres in a later stage. The method set grows as
// usecases arrive.
type Repository interface {
	Create(ctx context.Context, m *Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*Message, error)
	// ListByConversation returns messages ordered by CreatedAt ascending. When
	// after is non-nil, only messages newer than that message id are returned
	// (the reconnect catch-up from spec §4). limit caps the page size.
	ListByConversation(ctx context.Context, conversationID uuid.UUID, after *uuid.UUID, limit int) ([]*Message, error)
	// Update persists a mutated message (Edit / MarkDeleted).
	Update(ctx context.Context, m *Message) error
}
