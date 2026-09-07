// internal/infrastructure/postgres/conversation_repository_test.go
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

func within(t *testing.T, got, want time.Time, tol time.Duration) {
	t.Helper()
	if d := got.Sub(want); d < -tol || d > tol {
		t.Errorf("time %v not within %v of %v (delta %v)", got, tol, want, d)
	}
}

func TestConversationRepository_CreateGroupChatAndGetByID(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Team Standup")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("ID = %v, want %v", got.ID, c.ID)
	}
	if got.Kind != conversation.KindGroupChat {
		t.Errorf("Kind = %q, want %q", got.Kind, conversation.KindGroupChat)
	}
	if got.Title != "Team Standup" {
		t.Errorf("Title = %q, want %q", got.Title, "Team Standup")
	}
	within(t, got.CreatedAt, c.CreatedAt, time.Second)
}

func TestConversationRepository_GetByID_NotFound(t *testing.T) {
	repo := NewConversationRepository(testDB)
	_, err := repo.GetByID(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestConversationRepository_GetMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Team")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m, err := repo.GetMember(ctx, c.ID, creator.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if m.Role != conversation.RoleOwner {
		t.Errorf("Role = %q, want %q", m.Role, conversation.RoleOwner)
	}
	if m.ConversationID != c.ID || m.UserID != creator.ID {
		t.Errorf("member ids = (%v,%v), want (%v,%v)", m.ConversationID, m.UserID, c.ID, creator.ID)
	}
	if m.MutedUntil != nil {
		t.Errorf("MutedUntil = %v, want nil", m.MutedUntil)
	}
	if m.LastReadMessageID != nil {
		t.Errorf("LastReadMessageID = %v, want nil", m.LastReadMessageID)
	}
	if m.JoinedAt.IsZero() {
		t.Error("JoinedAt is zero")
	}
}

func TestConversationRepository_GetMember_NotMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)
	other := createTestUser(t)

	c, members, _ := conversation.NewGroupChat(creator.ID, "Team")
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.GetMember(ctx, c.ID, other.ID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}
}

func TestConversationRepository_CreateDirect_BothMembers(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	a := createTestUser(t)
	b := createTestUser(t)

	c, members, err := conversation.NewDirect(a.ID, b.ID)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, uid := range []uuid.UUID{a.ID, b.ID} {
		m, err := repo.GetMember(ctx, c.ID, uid)
		if err != nil {
			t.Fatalf("GetMember(%v): %v", uid, err)
		}
		if m.Role != conversation.RoleMember {
			t.Errorf("Role = %q, want %q", m.Role, conversation.RoleMember)
		}
	}
}

func TestConversationRepository_Create_RollsBackOnBadMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Rollback")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	members = append(members, &conversation.Member{
		ConversationID: c.ID,
		UserID:         uuid.Must(uuid.NewV7()), // not a real user -> FK violation
		Role:           conversation.RoleMember,
		JoinedAt:       time.Now().UTC(),
	})

	if err := repo.Create(ctx, c, members); err == nil {
		t.Fatal("Create with a non-existent member must return an error")
	}

	if _, err := repo.GetByID(ctx, c.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("expected the tx to roll back leaving no orphan conversation row (ErrNotFound), got %v", err)
	}
}

func TestConversationRepository_GetMember_NonNilOptionalFields(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Optional Fields")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// last_read_message_id has an FK to messages(id), so it must point at a real
	// row; create one to mark as read.
	marker, _ := message.NewText(c.ID, creator.ID, "read marker", nil)
	if err := NewMessageRepository(testDB).Create(ctx, marker); err != nil {
		t.Fatalf("Create marker message: %v", err)
	}

	mutedUntil := time.Now().UTC().Add(24 * time.Hour)
	if _, err := testDB.ExecContext(ctx, `
		UPDATE conversation_members
		SET muted_until = $1, last_read_message_id = $2
		WHERE conversation_id = $3 AND user_id = $4`,
		mutedUntil, marker.ID, c.ID, creator.ID,
	); err != nil {
		t.Fatalf("update member optional fields: %v", err)
	}

	m, err := repo.GetMember(ctx, c.ID, creator.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if m.MutedUntil == nil {
		t.Fatal("MutedUntil is nil, want non-nil")
	}
	within(t, *m.MutedUntil, mutedUntil, time.Second)
	if m.LastReadMessageID == nil {
		t.Fatal("LastReadMessageID is nil, want non-nil")
	}
	if *m.LastReadMessageID != marker.ID {
		t.Errorf("LastReadMessageID = %v, want %v", *m.LastReadMessageID, marker.ID)
	}
}

func TestConversationRepository_ListForUser(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	me := createTestUser(t)
	other := createTestUser(t)

	c1, m1, _ := conversation.NewGroupChat(me.ID, "First")
	if err := repo.Create(ctx, c1, m1); err != nil {
		t.Fatalf("Create c1: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	c2, m2, _ := conversation.NewGroupChat(me.ID, "Second")
	if err := repo.Create(ctx, c2, m2); err != nil {
		t.Fatalf("Create c2: %v", err)
	}
	cOther, mOther, _ := conversation.NewGroupChat(other.ID, "NotMine")
	if err := repo.Create(ctx, cOther, mOther); err != nil {
		t.Fatalf("Create cOther: %v", err)
	}

	list, err := repo.ListForUser(ctx, me.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != c2.ID || list[1].ID != c1.ID {
		t.Errorf("order = [%v, %v], want newest-first [%v, %v]", list[0].ID, list[1].ID, c2.ID, c1.ID)
	}
}
