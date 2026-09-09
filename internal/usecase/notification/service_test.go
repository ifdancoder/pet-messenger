package notification

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/domain/notification"
)

// fakeNotifRepo captures the args it is called with and returns canned results.
type fakeNotifRepo struct {
	gotUserID   uuid.UUID
	gotBefore   time.Time
	gotBeforeID uuid.UUID
	gotLimit    int
	list        []*domainnotif.Notification

	markUserID    uuid.UUID
	markThroughTS time.Time
	markThroughID uuid.UUID
	markResult    int64

	countUserID uuid.UUID
	countResult int
}

func (f *fakeNotifRepo) Create(context.Context, *domainnotif.Notification) error { return nil }

func (f *fakeNotifRepo) ListForUser(_ context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error) {
	f.gotUserID, f.gotBefore, f.gotBeforeID, f.gotLimit = userID, before, beforeID, limit
	return f.list, nil
}

func (f *fakeNotifRepo) MarkReadThrough(_ context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
	f.markUserID, f.markThroughTS, f.markThroughID = userID, throughCreatedAt, throughID
	return f.markResult, nil
}

func (f *fakeNotifRepo) CountUnread(_ context.Context, userID uuid.UUID) (int, error) {
	f.countUserID = userID
	return f.countResult, nil
}

func TestPageLimit(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{-1, DefaultPageLimit}, {0, DefaultPageLimit}, {1, 1},
		{MaxPageLimit, MaxPageLimit}, {MaxPageLimit + 1, MaxPageLimit},
	} {
		if got := PageLimit(tc.in); got != tc.want {
			t.Errorf("PageLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestList_ClampsLimitAndPassesArgsThrough(t *testing.T) {
	repo := &fakeNotifRepo{list: []*domainnotif.Notification{{ID: uuid.Must(uuid.NewV7())}}}
	s := NewService(repo)
	me := uuid.Must(uuid.NewV7())
	before := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	beforeID := uuid.Must(uuid.NewV7())

	for _, tc := range []struct{ in, want int }{{0, DefaultPageLimit}, {-3, DefaultPageLimit}, {1000, MaxPageLimit}, {15, 15}} {
		got, err := s.List(context.Background(), me, before, beforeID, tc.in)
		if err != nil {
			t.Fatalf("List(%d): %v", tc.in, err)
		}
		if repo.gotLimit != tc.want {
			t.Errorf("limit %d -> repo got %d, want %d", tc.in, repo.gotLimit, tc.want)
		}
		if len(got) != 1 {
			t.Errorf("List returned %d rows, want 1", len(got))
		}
	}
	if repo.gotUserID != me || !repo.gotBefore.Equal(before) || repo.gotBeforeID != beforeID {
		t.Errorf("repo got (user=%v before=%v id=%v)", repo.gotUserID, repo.gotBefore, repo.gotBeforeID)
	}
}

func TestMarkReadThrough_PassesThrough(t *testing.T) {
	repo := &fakeNotifRepo{markResult: 7}
	s := NewService(repo)
	me := uuid.Must(uuid.NewV7())
	ts := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	id := uuid.Must(uuid.NewV7())

	n, err := s.MarkReadThrough(context.Background(), me, ts, id)
	if err != nil {
		t.Fatalf("MarkReadThrough: %v", err)
	}
	if n != 7 {
		t.Errorf("marked = %d, want 7", n)
	}
	if repo.markUserID != me || !repo.markThroughTS.Equal(ts) || repo.markThroughID != id {
		t.Errorf("repo got (user=%v ts=%v id=%v)", repo.markUserID, repo.markThroughTS, repo.markThroughID)
	}
}

func TestUnreadCount_PassesThrough(t *testing.T) {
	repo := &fakeNotifRepo{countResult: 3}
	s := NewService(repo)
	me := uuid.Must(uuid.NewV7())

	got, err := s.UnreadCount(context.Background(), me)
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if got != 3 {
		t.Errorf("UnreadCount = %d, want 3", got)
	}
	if repo.countUserID != me {
		t.Errorf("repo got user %v, want %v", repo.countUserID, me)
	}
}
