// Package domain holds the Message aggregate root: chat/channel messages of
// several kinds, keyed by Type. Message owns its own state transitions (Edit,
// MarkDeleted); the application layer orchestrates, it does not reimplement
// them. It is the Entities layer (Clean Architecture) / domain core
// (Hexagonal) for the message bounded context — no dependency on anything
// outside the standard library, uuid, and time.
package domain

import (
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

// SystemEvent classifies a system message that records a membership or
// conversation-lifecycle change. It is "" for every non-system message.
type SystemEvent string

// System events.
const (
	SystemMemberJoined           SystemEvent = "member_joined"
	SystemMemberLeft             SystemEvent = "member_left"
	SystemMemberRemoved          SystemEvent = "member_removed"
	SystemMemberTierChanged      SystemEvent = "member_tier_changed"
	SystemConversationArchived   SystemEvent = "conversation_archived"
	SystemConversationUnarchived SystemEvent = "conversation_unarchived"
	SystemCallStarted            SystemEvent = "call_started"
	SystemCallEnded              SystemEvent = "call_ended"
)

// Valid reports whether e is one of the known system events.
func (e SystemEvent) Valid() bool {
	switch e {
	case SystemMemberJoined, SystemMemberLeft, SystemMemberRemoved,
		SystemMemberTierChanged, SystemConversationArchived, SystemConversationUnarchived,
		SystemCallStarted, SystemCallEnded:
		return true
	default:
		return false
	}
}

// maxContentRunes bounds message content length, measured in runes.
const maxContentRunes = 4096

// Encrypted messages carry one wrapped content key per trusted recipient
// device. Their wire representation is consequently larger than the original
// plaintext, especially in group chats. The server treats it as opaque data
// but still applies a hard bound to protect storage and realtime fan-out.
const (
	encryptedContentPrefix   = "e2ee:v1:"
	maxEncryptedContentBytes = 256 * 1024
)

// Domain errors.
var (
	ErrEmptyContent        = errors.New("message: content must not be empty")
	ErrInvalidSender       = errors.New("message: sender must be a valid user id")
	ErrInvalidConversation = errors.New("message: conversation must be a valid conversation id")
	ErrNotEditable         = errors.New("message: only a non-deleted text message can be edited")
	ErrNotFound            = errors.New("message: not found")
	// ErrInvalidLimit is returned by a history port for limit <= 0.
	ErrInvalidLimit = errors.New("message: limit must be a positive integer")
	// ErrConversationNotFound is returned by MessageWriter.Create when the
	// target conversation does not exist.
	ErrConversationNotFound = errors.New("message: conversation does not exist")
	// ErrDuplicateIdempotencyKey is returned by MessageWriter.Create when a
	// message with the same (ConversationID, SenderID, IdempotencyKey) already
	// exists. The whole Create transaction rolls back, so no seq is consumed.
	// SendMessage resolves it by returning the original message via
	// MessageLoader.GetByIdempotencyKey.
	ErrDuplicateIdempotencyKey = errors.New("message: a message with this idempotency key already exists in the conversation")
	// ErrInvalidSystemEvent is returned by NewSystemEvent for an unknown event.
	ErrInvalidSystemEvent = errors.New("message: unknown system event")
)

// ErrContentTooLong is reported when trimmed content exceeds maxContentRunes. It
// is built from the same constant so the limit is stated in exactly one place.
var ErrContentTooLong = fmt.Errorf("message: content must be at most %d runes", maxContentRunes)

// Message is a single message in a conversation. Seq is the per-conversation
// monotonic sequence number assigned by MessageWriter.Create (0 until
// persisted); it is the ordering and pagination key. The sequence is
// contiguous only while messages are never hard-deleted (today they are only
// soft-deleted via MarkDeleted); a future hard delete would leave a permanent
// gap. SenderID is nil for system and call_event messages. ReplyToID,
// EditedAt, and DeletedAt are nil until set. A text message read back from
// storage may also have a nil SenderID if the author's account was later
// deleted (sender_id is ON DELETE SET NULL); consumers rendering a text
// message must handle that.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Seq            int64
	SenderID       *uuid.UUID
	Type           Type
	Content        string
	// SystemEvent is set (non-"") only on system messages that record a
	// membership / lifecycle change; SystemTargetID is the member the event is
	// about (nil for conversation-level events). Both are nil/"" for text.
	SystemEvent    SystemEvent
	SystemTargetID *uuid.UUID
	ReplyToID      *uuid.UUID
	// IdempotencyKey is the optional client-supplied send-dedup token. It is
	// written by MessageWriter.Create and is NOT repopulated on reads (it is a
	// write-side hint, never rendered to clients), so it is nil on any Message
	// returned by a load/history method.
	IdempotencyKey *string
	EditedAt       *time.Time
	DeletedAt      *time.Time
	CreatedAt      time.Time
	// ReadBy and RecipientCount are populated for history responses. They are
	// derived from member read cursors and are never persisted with the message.
	ReadBy         int
	RecipientCount int
}

// validateContent trims surrounding whitespace and enforces the 1..4096-rune
// bound. It returns the trimmed content.
func validateContent(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	switch {
	case c == "":
		return "", ErrEmptyContent
	case strings.HasPrefix(c, encryptedContentPrefix) && len(c) > maxEncryptedContentBytes:
		return "", ErrContentTooLong
	case !strings.HasPrefix(c, encryptedContentPrefix) && utf8.RuneCountInString(c) > maxContentRunes:
		return "", ErrContentTooLong
	default:
		return c, nil
	}
}

// NewText builds a text message from senderID in conversationID. content is
// trimmed and must be 1..4096 runes. replyToID is optional and passed through
// as-is (existence is an application/port concern).
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

const maxSystemDetailBytes = 64

// NewSystemEvent builds a system message recording a membership or
// conversation-lifecycle change. actorID is the user who caused it (nil for
// system-initiated events such as auto-succession). targetID is the member the
// event concerns (uuid.Nil for conversation-level events). detail is an
// optional short token — for member_tier_changed the resulting tier
// ("creator_admin", "admin", "" for a demotion to no tier); "" otherwise. It is
// NOT run through validateContent.
func NewSystemEvent(conversationID uuid.UUID, actorID *uuid.UUID, event SystemEvent, targetID uuid.UUID, detail string) (*Message, error) {
	if conversationID == uuid.Nil {
		return nil, ErrInvalidConversation
	}
	if !event.Valid() {
		return nil, ErrInvalidSystemEvent
	}
	if len(detail) > maxSystemDetailBytes {
		return nil, ErrContentTooLong
	}
	var target *uuid.UUID
	if targetID != uuid.Nil {
		t := targetID
		target = &t
	}
	var sender *uuid.UUID
	if actorID != nil && *actorID != uuid.Nil {
		s := *actorID
		sender = &s
	}
	return &Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: conversationID,
		SenderID:       sender,
		Type:           TypeSystem,
		Content:        detail,
		SystemEvent:    event,
		SystemTargetID: target,
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
