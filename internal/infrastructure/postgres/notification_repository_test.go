package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/notification"
)

func mkNotif(t *testing.T, userID uuid.UUID, dedup string) *notification.Notification {
	t.Helper()
	n, err := notification.New(userID, notification.KindMessage, json.RawMessage(`{"k":1}`), dedup)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNotificationRepository_CreateAndDedup(t *testing.T) {
	repo := NewNotificationRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	n1 := mkNotif(t, u.ID, "msg:abc")
	if err := repo.Create(ctx, n1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	n2 := mkNotif(t, u.ID, "msg:abc")
	if err := repo.Create(ctx, n2); !errors.Is(err, notification.ErrDuplicate) {
		t.Fatalf("Create dup = %v, want ErrDuplicate", err)
	}
	// same dedup for a DIFFERENT user is fine
	u2 := createTestUser(t)
	if err := repo.Create(ctx, mkNotif(t, u2.ID, "msg:abc")); err != nil {
		t.Fatalf("Create for other user: %v", err)
	}
	// empty dedup never collides
	if err := repo.Create(ctx, mkNotif(t, u.ID, "")); err != nil {
		t.Fatalf("Create keyless: %v", err)
	}
	if err := repo.Create(ctx, mkNotif(t, u.ID, "")); err != nil {
		t.Fatalf("Create keyless #2: %v", err)
	}
}

func TestNotificationRepository_ListMarkCount(t *testing.T) {
	repo := NewNotificationRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	var made []*notification.Notification
	for i := 0; i < 3; i++ {
		n := mkNotif(t, u.ID, "")
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("seed #%d: %v", i, err)
		}
		made = append(made, n)
		time.Sleep(2 * time.Millisecond)
	}
	// made[2] is newest.

	page, err := repo.ListForUser(ctx, u.ID, time.Time{}, uuid.Nil, 10)
	if err != nil || len(page) != 3 || page[0].ID != made[2].ID || page[2].ID != made[0].ID {
		t.Fatalf("ListForUser = %v, %v", idsOfNotif(page), err)
	}
	if c, _ := repo.CountUnread(ctx, u.ID); c != 3 {
		t.Fatalf("unread = %d, want 3", c)
	}

	// mark read through the middle one
	mid := made[1]
	n, err := repo.MarkReadThrough(ctx, u.ID, mid.CreatedAt, mid.ID)
	if err != nil || n != 2 {
		t.Fatalf("MarkReadThrough = %d, %v, want 2", n, err)
	}
	if c, _ := repo.CountUnread(ctx, u.ID); c != 1 {
		t.Fatalf("unread after = %d, want 1", c)
	}
	// idempotent-ish: re-marking the same range updates 0 (already read)
	n, _ = repo.MarkReadThrough(ctx, u.ID, mid.CreatedAt, mid.ID)
	if n != 0 {
		t.Fatalf("re-mark = %d, want 0", n)
	}
	// zero cursor marks the rest
	n, _ = repo.MarkReadThrough(ctx, u.ID, time.Time{}, uuid.Nil)
	if n != 1 {
		t.Fatalf("mark all = %d, want 1", n)
	}

	// keyset paging
	p1, _ := repo.ListForUser(ctx, u.ID, time.Time{}, uuid.Nil, 2)
	if len(p1) != 2 {
		t.Fatalf("p1 len = %d", len(p1))
	}
	last := p1[len(p1)-1]
	p2, _ := repo.ListForUser(ctx, u.ID, last.CreatedAt, last.ID, 2)
	if len(p2) != 1 || p2[0].ID != made[0].ID {
		t.Fatalf("p2 = %v", idsOfNotif(p2))
	}
}

func idsOfNotif(ns []*notification.Notification) []uuid.UUID {
	out := make([]uuid.UUID, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}
