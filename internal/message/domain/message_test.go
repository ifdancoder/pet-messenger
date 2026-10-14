package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/message/domain"
)

// newText builds a valid text message or fails the test.
func newText(t *testing.T) *domain.Message {
	t.Helper()
	m, err := domain.NewText(uuid.New(), uuid.New(), "hi", nil)
	if err != nil {
		t.Fatalf("NewText: unexpected error: %v", err)
	}
	return m
}

// newSystem builds a valid system message or fails the test.
func newSystem(t *testing.T) *domain.Message {
	t.Helper()
	m, err := domain.NewSystem(uuid.New(), "alice joined")
	if err != nil {
		t.Fatalf("NewSystem: unexpected error: %v", err)
	}
	return m
}

func TestTypeValid(t *testing.T) {
	tests := []struct {
		typ  domain.Type
		want bool
	}{
		{domain.TypeText, true},
		{domain.TypeAttachment, true},
		{domain.TypeSystem, true},
		{domain.TypeCallEvent, true},
		{domain.Type("bogus"), false},
		{domain.Type(""), false},
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

	m, err := domain.NewText(conv, sender, "  hello world  ", &reply)
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
	if m.Type != domain.TypeText {
		t.Errorf("Type = %q, want %q", m.Type, domain.TypeText)
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
	if m.IdempotencyKey != nil {
		t.Errorf("IdempotencyKey should be nil on a new message, got %v", *m.IdempotencyKey)
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
	m, err := domain.NewText(uuid.New(), uuid.New(), "hi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ReplyToID != nil {
		t.Errorf("ReplyToID = %v, want nil", m.ReplyToID)
	}
}

func TestNewTextNilValuedReply(t *testing.T) {
	nilReply := uuid.Nil
	m, err := domain.NewText(uuid.New(), uuid.New(), "hi", &nilReply)
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
		{"nil conversation", uuid.Nil, sender, "hi", domain.ErrInvalidConversation},
		{"nil sender", conv, uuid.Nil, "hi", domain.ErrInvalidSender},
		{"empty content", conv, sender, "", domain.ErrEmptyContent},
		{"whitespace-only content", conv, sender, "   \t\n ", domain.ErrEmptyContent},
		{"content too long", conv, sender, strings.Repeat("x", 4097), domain.ErrContentTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewText(tt.conv, tt.sender, tt.content, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewTextContentBoundary(t *testing.T) {
	// 4096 runes of a 2-byte rune: passes on a rune count, would fail on bytes.
	content := strings.Repeat("я", 4096)
	m, err := domain.NewText(uuid.New(), uuid.New(), content, nil)
	if err != nil {
		t.Fatalf("4096-rune content should be valid, got %v", err)
	}
	if m.Content != content {
		t.Error("Content should be stored verbatim when already trimmed")
	}
}

func TestNewTextAllowsOpaqueEncryptedEnvelopeOverPlaintextLimit(t *testing.T) {
	content := "e2ee:v1:" + strings.Repeat("x", 8_000)
	m, err := domain.NewText(uuid.New(), uuid.New(), content, nil)
	if err != nil {
		t.Fatalf("encrypted envelope should use its own wire-size limit: %v", err)
	}
	if m.Content != content {
		t.Fatal("encrypted envelope was changed")
	}

	tooLarge := "e2ee:v1:" + strings.Repeat("x", 256*1024)
	if _, err := domain.NewText(uuid.New(), uuid.New(), tooLarge, nil); !errors.Is(err, domain.ErrContentTooLong) {
		t.Fatalf("oversized encrypted envelope error = %v, want ErrContentTooLong", err)
	}
}

func TestNewSystem(t *testing.T) {
	conv := uuid.New()

	m, err := domain.NewSystem(conv, "alice joined the group")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != domain.TypeSystem {
		t.Errorf("Type = %q, want %q", m.Type, domain.TypeSystem)
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
	if _, err := domain.NewSystem(uuid.New(), "  "); !errors.Is(err, domain.ErrEmptyContent) {
		t.Fatalf("err = %v, want %v", err, domain.ErrEmptyContent)
	}
	if _, err := domain.NewSystem(uuid.Nil, "hi"); !errors.Is(err, domain.ErrInvalidConversation) {
		t.Fatalf("err = %v, want %v", err, domain.ErrInvalidConversation)
	}
}

func TestSystemEvent_Valid(t *testing.T) {
	for _, e := range []domain.SystemEvent{
		domain.SystemMemberJoined, domain.SystemMemberLeft, domain.SystemMemberRemoved,
		domain.SystemMemberTierChanged, domain.SystemConversationArchived, domain.SystemConversationUnarchived,
	} {
		if !e.Valid() {
			t.Errorf("%q should be Valid", e)
		}
	}
	if domain.SystemEvent("member_banned").Valid() {
		t.Error(`SystemEvent("member_banned") should be invalid`)
	}
}

func TestSystemEvent_Valid_CallEvents(t *testing.T) {
	for _, e := range []domain.SystemEvent{domain.SystemCallStarted, domain.SystemCallEnded} {
		if !e.Valid() {
			t.Errorf("%q should be Valid", e)
		}
	}
}

func TestNewSystemEvent(t *testing.T) {
	conv := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())

	m, err := domain.NewSystemEvent(conv, &actor, domain.SystemMemberRemoved, target, "")
	if err != nil {
		t.Fatalf("NewSystemEvent: %v", err)
	}
	if m.Type != domain.TypeSystem || m.SystemEvent != domain.SystemMemberRemoved {
		t.Fatalf("m = %+v", m)
	}
	if m.SenderID == nil || *m.SenderID != actor {
		t.Fatalf("SenderID = %v, want %v", m.SenderID, actor)
	}
	if m.SystemTargetID == nil || *m.SystemTargetID != target {
		t.Fatalf("SystemTargetID = %v, want %v", m.SystemTargetID, target)
	}
	if m.ID.Version() != 7 || m.CreatedAt.IsZero() {
		t.Fatalf("id/timestamp: %+v", m)
	}

	// system-initiated: nil actor -> nil SenderID
	m, err = domain.NewSystemEvent(conv, nil, domain.SystemMemberTierChanged, target, "creator_admin")
	if err != nil {
		t.Fatalf("NewSystemEvent(nil actor): %v", err)
	}
	if m.SenderID != nil {
		t.Fatalf("SenderID = %v, want nil", m.SenderID)
	}
	if m.Content != "creator_admin" {
		t.Fatalf("Content = %q, want the tier token", m.Content)
	}

	// a non-nil pointer to uuid.Nil is also treated as system-initiated
	nilActor := uuid.Nil
	m, err = domain.NewSystemEvent(conv, &nilActor, domain.SystemMemberTierChanged, target, "creator_admin")
	if err != nil {
		t.Fatalf("NewSystemEvent(&uuid.Nil actor): %v", err)
	}
	if m.SenderID != nil {
		t.Fatalf("SenderID = %v, want nil for a &uuid.Nil actor", m.SenderID)
	}

	// conversation-level: uuid.Nil target -> nil SystemTargetID
	m, err = domain.NewSystemEvent(conv, &actor, domain.SystemConversationArchived, uuid.Nil, "")
	if err != nil {
		t.Fatalf("NewSystemEvent(conversation-level): %v", err)
	}
	if m.SystemTargetID != nil {
		t.Fatalf("SystemTargetID = %v, want nil", m.SystemTargetID)
	}

	// errors
	if _, err := domain.NewSystemEvent(uuid.Nil, &actor, domain.SystemMemberJoined, target, ""); !errors.Is(err, domain.ErrInvalidConversation) {
		t.Fatalf("nil conv: err = %v", err)
	}
	if _, err := domain.NewSystemEvent(conv, &actor, domain.SystemEvent("nope"), target, ""); !errors.Is(err, domain.ErrInvalidSystemEvent) {
		t.Fatalf("bad event: err = %v", err)
	}
	if _, err := domain.NewSystemEvent(conv, &actor, domain.SystemMemberJoined, target, strings.Repeat("x", 65)); !errors.Is(err, domain.ErrContentTooLong) {
		t.Fatalf("long detail: err = %v", err)
	}
}

func TestErrInvalidLimitIsDistinct(t *testing.T) {
	if errors.Is(domain.ErrInvalidLimit, domain.ErrNotFound) {
		t.Error("ErrInvalidLimit must not alias ErrNotFound")
	}
	if domain.ErrInvalidLimit == nil {
		t.Error("ErrInvalidLimit must be a non-nil sentinel")
	}
}

func TestErrConversationNotFoundIsDistinct(t *testing.T) {
	if domain.ErrConversationNotFound == nil {
		t.Error("ErrConversationNotFound must be a non-nil sentinel")
	}
	if errors.Is(domain.ErrConversationNotFound, domain.ErrNotFound) {
		t.Error("ErrConversationNotFound must not alias ErrNotFound")
	}
	if errors.Is(domain.ErrConversationNotFound, domain.ErrInvalidLimit) {
		t.Error("ErrConversationNotFound must not alias ErrInvalidLimit")
	}
}

func TestIsEditedIsDeleted(t *testing.T) {
	m, err := domain.NewText(uuid.New(), uuid.New(), "hi", nil)
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
	m, err := domain.NewText(uuid.New(), uuid.New(), "original", nil)
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
		if err := m.Edit("nope", now); !errors.Is(err, domain.ErrNotEditable) {
			t.Fatalf("err = %v, want %v", err, domain.ErrNotEditable)
		}
	})

	t.Run("deleted message", func(t *testing.T) {
		m := newText(t)
		m.MarkDeleted(now)
		if err := m.Edit("resurrect", now); !errors.Is(err, domain.ErrNotEditable) {
			t.Fatalf("err = %v, want %v", err, domain.ErrNotEditable)
		}
	})

	t.Run("invalid new content", func(t *testing.T) {
		m := newText(t)
		if err := m.Edit("   ", now); !errors.Is(err, domain.ErrEmptyContent) {
			t.Fatalf("err = %v, want %v", err, domain.ErrEmptyContent)
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
		if err := m.Edit(strings.Repeat("x", 4097), now); !errors.Is(err, domain.ErrContentTooLong) {
			t.Fatalf("err = %v, want %v", err, domain.ErrContentTooLong)
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
	m, err := domain.NewText(uuid.New(), uuid.New(), "secret", nil)
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
