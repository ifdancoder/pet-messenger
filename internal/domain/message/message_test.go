package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/message"
)

// newText builds a valid text message or fails the test.
func newText(t *testing.T) *message.Message {
	t.Helper()
	m, err := message.NewText(uuid.New(), uuid.New(), "hi", nil)
	if err != nil {
		t.Fatalf("NewText: unexpected error: %v", err)
	}
	return m
}

// newSystem builds a valid system message or fails the test.
func newSystem(t *testing.T) *message.Message {
	t.Helper()
	m, err := message.NewSystem(uuid.New(), "alice joined")
	if err != nil {
		t.Fatalf("NewSystem: unexpected error: %v", err)
	}
	return m
}

func TestTypeValid(t *testing.T) {
	tests := []struct {
		typ  message.Type
		want bool
	}{
		{message.TypeText, true},
		{message.TypeAttachment, true},
		{message.TypeSystem, true},
		{message.TypeCallEvent, true},
		{message.Type("bogus"), false},
		{message.Type(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			if got := tt.typ.Valid(); got != tt.want {
				t.Errorf("Type(%q).Valid() = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestNewText(t *testing.T) {
	conv, sender := uuid.New(), uuid.New()
	reply := uuid.New()

	m, err := message.NewText(conv, sender, "  hello world  ", &reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if m.ID.Version() != 7 {
		t.Errorf("ID version = %d, want 7 (UUIDv7)", m.ID.Version())
	}
	if m.ConversationID != conv {
		t.Errorf("ConversationID = %v, want %v", m.ConversationID, conv)
	}
	if m.SenderID == nil || *m.SenderID != sender {
		t.Errorf("SenderID = %v, want %v", m.SenderID, sender)
	}
	if m.Type != message.TypeText {
		t.Errorf("Type = %q, want %q", m.Type, message.TypeText)
	}
	if m.Content != "hello world" {
		t.Errorf("Content = %q, want %q (trimmed)", m.Content, "hello world")
	}
	if m.ReplyToID == nil || *m.ReplyToID != reply {
		t.Errorf("ReplyToID = %v, want %v", m.ReplyToID, reply)
	}
	if m.EditedAt != nil || m.DeletedAt != nil {
		t.Errorf("EditedAt/DeletedAt should be nil on a new message, got %v / %v", m.EditedAt, m.DeletedAt)
	}
	if m.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if m.CreatedAt.Location().String() != "UTC" {
		t.Errorf("CreatedAt not UTC: %v", m.CreatedAt.Location())
	}
	if m.Seq != 0 {
		t.Errorf("Seq = %d, want 0 for an unpersisted message", m.Seq)
	}
}

func TestNewTextNoReply(t *testing.T) {
	m, err := message.NewText(uuid.New(), uuid.New(), "hi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ReplyToID != nil {
		t.Errorf("ReplyToID = %v, want nil", m.ReplyToID)
	}
}

func TestNewTextNilValuedReply(t *testing.T) {
	nilReply := uuid.Nil
	m, err := message.NewText(uuid.New(), uuid.New(), "hi", &nilReply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ReplyToID != nil {
		t.Errorf("ReplyToID = %v, want nil for a nil-valued pointer", m.ReplyToID)
	}
}

func TestNewTextInvalid(t *testing.T) {
	conv, sender := uuid.New(), uuid.New()
	tests := []struct {
		name    string
		conv    uuid.UUID
		sender  uuid.UUID
		content string
		wantErr error
	}{
		{"nil conversation", uuid.Nil, sender, "hi", message.ErrInvalidConversation},
		{"nil sender", conv, uuid.Nil, "hi", message.ErrInvalidSender},
		{"empty content", conv, sender, "", message.ErrEmptyContent},
		{"whitespace-only content", conv, sender, "   \t\n ", message.ErrEmptyContent},
		{"content too long", conv, sender, strings.Repeat("x", 4097), message.ErrContentTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := message.NewText(tt.conv, tt.sender, tt.content, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewTextContentBoundary(t *testing.T) {
	// 4096 runes of a 2-byte rune: passes on a rune count, would fail on bytes.
	content := strings.Repeat("я", 4096)
	m, err := message.NewText(uuid.New(), uuid.New(), content, nil)
	if err != nil {
		t.Fatalf("4096-rune content should be valid, got %v", err)
	}
	if m.Content != content {
		t.Error("Content should be stored verbatim when already trimmed")
	}
}

func TestNewSystem(t *testing.T) {
	conv := uuid.New()

	m, err := message.NewSystem(conv, "alice joined the group")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != message.TypeSystem {
		t.Errorf("Type = %q, want %q", m.Type, message.TypeSystem)
	}
	if m.SenderID != nil {
		t.Errorf("SenderID = %v, want nil for a system message", m.SenderID)
	}
	if m.ConversationID != conv {
		t.Errorf("ConversationID = %v, want %v", m.ConversationID, conv)
	}
	if m.Content != "alice joined the group" {
		t.Errorf("Content = %q", m.Content)
	}
	if m.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if m.Seq != 0 {
		t.Errorf("Seq = %d, want 0 for an unpersisted message", m.Seq)
	}
}

func TestNewSystemInvalid(t *testing.T) {
	if _, err := message.NewSystem(uuid.New(), "  "); !errors.Is(err, message.ErrEmptyContent) {
		t.Fatalf("err = %v, want %v", err, message.ErrEmptyContent)
	}
	if _, err := message.NewSystem(uuid.Nil, "hi"); !errors.Is(err, message.ErrInvalidConversation) {
		t.Fatalf("err = %v, want %v", err, message.ErrInvalidConversation)
	}
}

func TestErrInvalidLimitIsDistinct(t *testing.T) {
	if errors.Is(message.ErrInvalidLimit, message.ErrNotFound) {
		t.Error("ErrInvalidLimit must not alias ErrNotFound")
	}
	if message.ErrInvalidLimit == nil {
		t.Error("ErrInvalidLimit must be a non-nil sentinel")
	}
}

func TestIsEditedIsDeleted(t *testing.T) {
	m, err := message.NewText(uuid.New(), uuid.New(), "hi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.IsEdited() {
		t.Error("new message should not be edited")
	}
	if m.IsDeleted() {
		t.Error("new message should not be deleted")
	}

	now := time.Now().UTC()
	m.EditedAt = &now
	if !m.IsEdited() {
		t.Error("IsEdited should be true once EditedAt is set")
	}
	m.DeletedAt = &now
	if !m.IsDeleted() {
		t.Error("IsDeleted should be true once DeletedAt is set")
	}
}

func TestEdit(t *testing.T) {
	m, err := message.NewText(uuid.New(), uuid.New(), "original", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now().UTC()

	if err := m.Edit("  updated text  ", now); err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}
	if m.Content != "updated text" {
		t.Errorf("Content = %q, want %q", m.Content, "updated text")
	}
	if m.EditedAt == nil || !m.EditedAt.Equal(now) {
		t.Errorf("EditedAt = %v, want %v", m.EditedAt, now)
	}
	if !m.IsEdited() {
		t.Error("IsEdited should be true after Edit")
	}
}

func TestEditRejected(t *testing.T) {
	now := time.Now().UTC()

	t.Run("system message", func(t *testing.T) {
		m := newSystem(t)
		if err := m.Edit("nope", now); !errors.Is(err, message.ErrNotEditable) {
			t.Fatalf("err = %v, want %v", err, message.ErrNotEditable)
		}
	})

	t.Run("deleted message", func(t *testing.T) {
		m := newText(t)
		m.MarkDeleted(now)
		if err := m.Edit("resurrect", now); !errors.Is(err, message.ErrNotEditable) {
			t.Fatalf("err = %v, want %v", err, message.ErrNotEditable)
		}
	})

	t.Run("invalid new content", func(t *testing.T) {
		m := newText(t)
		if err := m.Edit("   ", now); !errors.Is(err, message.ErrEmptyContent) {
			t.Fatalf("err = %v, want %v", err, message.ErrEmptyContent)
		}
		if m.IsEdited() {
			t.Error("a rejected Edit must not set EditedAt")
		}
		if m.Content != "hi" {
			t.Errorf("a rejected Edit must not change Content, got %q", m.Content)
		}
	})

	t.Run("new content too long", func(t *testing.T) {
		m := newText(t)
		if err := m.Edit(strings.Repeat("x", 4097), now); !errors.Is(err, message.ErrContentTooLong) {
			t.Fatalf("err = %v, want %v", err, message.ErrContentTooLong)
		}
		if m.IsEdited() {
			t.Error("a rejected Edit must not set EditedAt")
		}
		if m.Content != "hi" {
			t.Errorf("a rejected Edit must not change Content, got %q", m.Content)
		}
	})
}

func TestMarkDeleted(t *testing.T) {
	m, err := message.NewText(uuid.New(), uuid.New(), "secret", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now().UTC()

	m.MarkDeleted(now)
	if m.DeletedAt == nil || !m.DeletedAt.Equal(now) {
		t.Errorf("DeletedAt = %v, want %v", m.DeletedAt, now)
	}
	if m.Content != "" {
		t.Errorf("Content = %q, want cleared", m.Content)
	}
	if !m.IsDeleted() {
		t.Error("IsDeleted should be true after MarkDeleted")
	}

	later := now.Add(time.Minute)
	m.MarkDeleted(later)
	if !m.DeletedAt.Equal(now) {
		t.Errorf("second MarkDeleted must be a no-op keeping the first deletion time %v, got %v", now, m.DeletedAt)
	}
}
