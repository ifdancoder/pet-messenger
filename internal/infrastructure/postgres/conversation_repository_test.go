// internal/infrastructure/postgres/conversation_repository_test.go
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
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
