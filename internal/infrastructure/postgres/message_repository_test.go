// internal/infrastructure/postgres/message_repository_test.go
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

// seedConversation creates a group chat owned by a fresh user and returns the
// conversation id and the owner id.
func seedConversation(t *testing.T) (convID, ownerID uuid.UUID) {
	t.Helper()
	owner := createTestUser(t)
	c, members, err := conversation.NewGroupChat(owner.ID, "seed")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return c.ID, owner.ID
}

func TestMessageRepository_CreateTextAndGetByID(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, err := message.NewText(convID, sender, "  hello  ", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != m.ID || got.ConversationID != convID {
		t.Errorf("ids = (%v,%v), want (%v,%v)", got.ID, got.ConversationID, m.ID, convID)
	}
	if got.SenderID == nil || *got.SenderID != sender {
		t.Errorf("SenderID = %v, want %v", got.SenderID, sender)
	}
	if got.Type != message.TypeText {
		t.Errorf("Type = %q, want %q", got.Type, message.TypeText)
	}
	if got.Content != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
	if got.ReplyToID != nil || got.EditedAt != nil || got.DeletedAt != nil {
		t.Errorf("expected nil reply/edited/deleted, got %v/%v/%v", got.ReplyToID, got.EditedAt, got.DeletedAt)
	}
}

func TestMessageRepository_CreateSystem_NilSender(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, _ := seedConversation(t)

	m, err := message.NewSystem(convID, "alice joined")
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SenderID != nil {
		t.Errorf("SenderID = %v, want nil for a system message", got.SenderID)
	}
	if got.Type != message.TypeSystem {
		t.Errorf("Type = %q, want %q", got.Type, message.TypeSystem)
	}
}

func TestMessageRepository_CreateWithReplyTo(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	first, _ := message.NewText(convID, sender, "first", nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	reply, _ := message.NewText(convID, sender, "reply", &first.ID)
	if err := repo.Create(ctx, reply); err != nil {
		t.Fatalf("Create reply: %v", err)
	}

	got, err := repo.GetByID(ctx, reply.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ReplyToID == nil || *got.ReplyToID != first.ID {
		t.Errorf("ReplyToID = %v, want %v", got.ReplyToID, first.ID)
	}
}

func TestMessageRepository_GetByID_NotFound(t *testing.T) {
	repo := NewMessageRepository(testDB)
	_, err := repo.GetByID(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMessageRepository_Update_PersistsEditAndDelete(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := message.NewText(convID, sender, "original", nil)
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	editAt := time.Now().UTC()
	if err := m.Edit("edited body", editAt); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("Update after edit: %v", err)
	}
	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Content != "edited body" {
		t.Errorf("Content = %q, want %q", got.Content, "edited body")
	}
	if got.EditedAt == nil {
		t.Fatal("EditedAt is nil after Update")
	}
	within(t, *got.EditedAt, editAt, time.Second)

	delAt := time.Now().UTC()
	m.MarkDeleted(delAt)
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("Update after delete: %v", err)
	}
	got, err = repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("DeletedAt is nil after Update")
	}
	if got.Content != "" {
		t.Errorf("Content = %q, want cleared after delete", got.Content)
	}
}

func TestMessageRepository_Update_NotFound(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, sender := seedConversation(t)
	m, _ := message.NewText(convID, sender, "never stored", nil)
	if err := repo.Update(context.Background(), m); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMessageRepository_ListByConversation_Keyset(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	// 5 messages, oldest to newest, spaced so (created_at) is strictly ordered.
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		m, _ := message.NewText(convID, sender, "m", nil)
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create m%d: %v", i, err)
		}
		ids = append(ids, m.ID)
		time.Sleep(2 * time.Millisecond)
	}

	// after=nil, limit 3 -> first three, ascending
	page, err := repo.ListByConversation(ctx, convID, nil, 3)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page) != 3 || page[0].ID != ids[0] || page[2].ID != ids[2] {
		t.Fatalf("page1 = %v, want first three of %v", idsOf(page), ids)
	}

	// after=ids[2], limit 10 -> the remaining two
	page, err = repo.ListByConversation(ctx, convID, &ids[2], 10)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page) != 2 || page[0].ID != ids[3] || page[1].ID != ids[4] {
		t.Fatalf("page2 = %v, want %v", idsOf(page), []uuid.UUID{ids[3], ids[4]})
	}

	// after=last -> empty
	page, err = repo.ListByConversation(ctx, convID, &ids[4], 10)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("page3 = %v, want empty", idsOf(page))
	}

	// empty conversation -> empty, no error
	otherConv, _ := seedConversation(t)
	page, err = repo.ListByConversation(ctx, otherConv, nil, 10)
	if err != nil {
		t.Fatalf("empty conv: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("empty conv page = %v, want empty", idsOf(page))
	}
}

func TestMessageRepository_ListByConversation_IncludesSoftDeleted(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	keep, _ := message.NewText(convID, sender, "keep", nil)
	if err := repo.Create(ctx, keep); err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	gone, _ := message.NewText(convID, sender, "gone", nil)
	if err := repo.Create(ctx, gone); err != nil {
		t.Fatalf("Create gone: %v", err)
	}
	gone.MarkDeleted(time.Now().UTC())
	if err := repo.Update(ctx, gone); err != nil {
		t.Fatalf("Update gone: %v", err)
	}

	page, err := repo.ListByConversation(ctx, convID, nil, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len = %d, want 2 (soft-deleted rows are returned)", len(page))
	}
	if page[1].ID != gone.ID || !page[1].IsDeleted() || page[1].Content != "" {
		t.Errorf("deleted row = %+v, want id %v, IsDeleted, empty content", page[1], gone.ID)
	}
}

func TestMessageRepository_ListByConversation_InvalidLimit(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, _ := seedConversation(t)
	for _, lim := range []int{0, -1} {
		if _, err := repo.ListByConversation(context.Background(), convID, nil, lim); !errors.Is(err, message.ErrInvalidLimit) {
			t.Fatalf("limit %d: expected ErrInvalidLimit, got %v", lim, err)
		}
	}
}

func idsOf(ms []*message.Message) []uuid.UUID {
	out := make([]uuid.UUID, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}
