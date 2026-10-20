// internal/notification/application/query/list_notifications_test.go
package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/notification/application/port/in"
	domainnotif "pet-messenger/internal/notification/domain"
)

type fakeNotificationReader struct {
	gotUserID   uuid.UUID
	gotBefore   time.Time
	gotBeforeID uuid.UUID
	gotLimit    int
	list        []*domainnotif.Notification

	countResult int
}

func (f *fakeNotificationReader) ListForUser(_ context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error) {
	f.gotUserID, f.gotBefore, f.gotBeforeID, f.gotLimit = userID, before, beforeID, limit
	return f.list, nil
}

func (f *fakeNotificationReader) CountUnread(context.Context, uuid.UUID) (int, error) {
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

func TestListNotifications_ClampsLimitAndPassesArgsThrough(t *testing.T) {
	reader := &fakeNotificationReader{list: []*domainnotif.Notification{{ID: uuid.Must(uuid.NewV7())}}}
	h := NewListNotificationsHandler(reader)
	actor := uuid.Must(uuid.NewV7())
	before := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	beforeID := uuid.Must(uuid.NewV7())

	for _, tc := range []struct{ in, want int }{{0, DefaultPageLimit}, {-3, DefaultPageLimit}, {1000, MaxPageLimit}, {15, 15}} {
		got, err := h.Handle(context.Background(), in.ListNotificationsQuery{Actor: actor, Before: before, BeforeID: beforeID, Limit: tc.in})
		if err != nil {
			t.Fatalf("Handle(%d): %v", tc.in, err)
		}
		if reader.gotLimit != tc.want {
			t.Errorf("limit %d -> reader got %d, want %d", tc.in, reader.gotLimit, tc.want)
		}
		if len(got) != 1 {
			t.Errorf("Handle returned %d rows, want 1", len(got))
		}
	}
	if reader.gotUserID != actor || !reader.gotBefore.Equal(before) || reader.gotBeforeID != beforeID {
		t.Errorf("reader got (user=%v before=%v id=%v)", reader.gotUserID, reader.gotBefore, reader.gotBeforeID)
	}
}
