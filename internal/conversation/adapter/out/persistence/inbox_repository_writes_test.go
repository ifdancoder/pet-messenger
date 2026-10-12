// internal/conversation/adapter/out/persistence/inbox_repository_writes_test.go
package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

func TestInboxRepository_UpsertMember_IsIdempotent(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	c := mustCreateDirect(t, ctx, convRepo, userA, userB, time.Now())
	row := InboxRow{UserID: userA, ConversationID: c.ID, Kind: domain.KindDirect, CreatedAt: c.CreatedAt, DirectPeerUserID: &userB, EventSeq: 1}

	if err := repo.UpsertMember(ctx, row); err != nil {
		t.Fatalf("first UpsertMember() error = %v", err)
	}
	if err := repo.UpsertMember(ctx, row); err != nil { // redelivery
		t.Fatalf("second UpsertMember() error = %v", err)
	}

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows after duplicate upsert, want 1", len(got))
	}
}

func TestInboxRepository_UpdateArchived_IgnoresStaleSeq(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	c := mustCreateDirect(t, ctx, convRepo, userA, userB, time.Now())
	mustUpsertInboxRow(t, ctx, userA, c.ID, domain.KindDirect, c.CreatedAt, c.CreatedAt, &userB)

	// Apply seq=5 (archived), then a stale seq=3 unarchive must not win.
	now := time.Now().UTC()
	if err := repo.UpdateArchived(ctx, c.ID, &now, 5); err != nil {
		t.Fatalf("UpdateArchived(seq=5) error = %v", err)
	}
	if err := repo.UpdateArchived(ctx, c.ID, nil, 3); err != nil {
		t.Fatalf("UpdateArchived(seq=3) error = %v", err)
	}

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 0 { // the row is still archived, so ListInbox's WHERE archived_at IS NULL excludes it
		t.Fatalf("got %d rows, want 0 (row should still be archived)", len(got))
	}
}

func TestInboxRepository_RemoveMember_IgnoresStaleSeq(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	c := mustCreateDirect(t, ctx, convRepo, userA, userB, time.Now())
	mustUpsertInboxRow(t, ctx, userA, c.ID, domain.KindDirect, c.CreatedAt, c.CreatedAt, &userB)

	// Bump to seq=5 via an archive update, then a stale seq=2 removal must
	// not delete the row.
	now := time.Now().UTC()
	if err := repo.UpdateArchived(ctx, c.ID, &now, 5); err != nil {
		t.Fatalf("UpdateArchived(seq=5) error = %v", err)
	}
	if err := repo.UpdateArchived(ctx, c.ID, nil, 6); err != nil { // unarchive so ListInbox can see it again
		t.Fatalf("UpdateArchived(seq=6) error = %v", err)
	}
	if err := repo.RemoveMember(ctx, c.ID, userA, 2); err != nil {
		t.Fatalf("RemoveMember(seq=2) error = %v", err)
	}

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (stale removal must be ignored)", len(got))
	}
}

func TestInboxRepository_UpdateLastMessageAt_IgnoresOlderTimestamp(t *testing.T) {
	repo := NewInboxRepository(testDB)
	convRepo := NewConversationRepository(testDB)
	ctx := context.Background()

	userA, userB := createTestUser(t).ID, createTestUser(t).ID
	base := time.Now().UTC()
	c := mustCreateDirect(t, ctx, convRepo, userA, userB, base)
	mustUpsertInboxRow(t, ctx, userA, c.ID, domain.KindDirect, c.CreatedAt, base, &userB)

	newer := base.Add(time.Hour)
	if err := repo.UpdateLastMessageAt(ctx, c.ID, newer); err != nil {
		t.Fatalf("UpdateLastMessageAt(newer) error = %v", err)
	}
	older := base.Add(-time.Hour)
	if err := repo.UpdateLastMessageAt(ctx, c.ID, older); err != nil {
		t.Fatalf("UpdateLastMessageAt(older) error = %v", err)
	}

	got, err := repo.ListInbox(ctx, userA, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	within(t, got[0].LastMessageAt, newer, time.Millisecond) // Postgres rounds to microseconds
}
