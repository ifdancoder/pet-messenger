// internal/conversation/adapter/out/persistence/inbox_repository_test.go
package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

func mustCreateDirect(t *testing.T, ctx context.Context, repo *ConversationRepository, a, b uuid.UUID, createdAt time.Time) *domain.Conversation {
	t.Helper()
	c, members, err := domain.NewDirect(a, b)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	c.CreatedAt, c.LastMessageAt = createdAt, createdAt
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c
}

func mustUpsertInboxRow(t *testing.T, ctx context.Context, userID, conversationID uuid.UUID, kind domain.Kind, createdAt, lastMessageAt time.Time, peer *uuid.UUID) {
	t.Helper()
	_, err := testDB.ExecContext(ctx, `
		INSERT INTO conversation_inbox (user_id, conversation_id, kind, created_at, last_message_at, direct_peer_user_id, last_event_seq)
		VALUES ($1, $2, $3::conversation_kind, $4, $5, $6, 1)`,
		userID, conversationID, string(kind), createdAt, lastMessageAt, nullableUUID(peer))
	if err != nil {
		t.Fatalf("insert inbox row: %v", err)
	}
}

func TestInboxRepository_ListInbox_OrdersByActivityDesc(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	userC := createTestUser(t).ID
	older := mustCreateDirect(t, ctx, convRepo, userA, userB, time.Now().Add(-time.Hour))
	newer := mustCreateDirect(t, ctx, convRepo, userA, userC, time.Now())

	mustUpsertInboxRow(t, ctx, userA, older.ID, domain.KindDirect, older.CreatedAt, older.CreatedAt, &userB)
	mustUpsertInboxRow(t, ctx, userA, newer.ID, domain.KindDirect, newer.CreatedAt, newer.CreatedAt, &userC)

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 2 || got[0].ConversationID != newer.ID || got[1].ConversationID != older.ID {
		t.Fatalf("got %d entries in wrong order: %+v", len(got), got)
	}
}

func TestInboxRepository_ListInbox_ResolvesDirectPeerLive(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA := createTestUser(t).ID
	peer := createTestUser(t)
	c := mustCreateDirect(t, ctx, convRepo, userA, peer.ID, time.Now())
	mustUpsertInboxRow(t, ctx, userA, c.ID, domain.KindDirect, c.CreatedAt, c.CreatedAt, &peer.ID)

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].DirectUsername != peer.Username {
		t.Fatalf("DirectUsername = %q, want %q", got[0].DirectUsername, peer.Username)
	}
}

func TestInboxRepository_ListInbox_ExcludesArchived(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	c := mustCreateDirect(t, ctx, convRepo, userA, userB, time.Now())
	mustUpsertInboxRow(t, ctx, userA, c.ID, domain.KindDirect, c.CreatedAt, c.CreatedAt, &userB)

	now := time.Now().UTC()
	if _, err := testDB.ExecContext(ctx, `UPDATE conversation_inbox SET archived_at = $1 WHERE user_id = $2 AND conversation_id = $3`, now, userA, c.ID); err != nil {
		t.Fatalf("archive inbox row: %v", err)
	}

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 (archived)", len(got))
	}
}
