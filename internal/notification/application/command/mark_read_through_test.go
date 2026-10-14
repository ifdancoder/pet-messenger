// internal/notification/application/command/mark_read_through_test.go
package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/notification/application/port/in"
)

type fakeNotificationMarker struct {
	markReadThrough func(userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error)
}

func (f *fakeNotificationMarker) MarkReadThrough(_ context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
	return f.markReadThrough(userID, throughCreatedAt, throughID)
}

func TestMarkReadThrough_PassesThrough(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	ts := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	id := uuid.Must(uuid.NewV7())
	var gotUser uuid.UUID
	var gotTS time.Time
	var gotID uuid.UUID
	h := NewMarkReadThroughHandler(&fakeNotificationMarker{
		markReadThrough: func(userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
			gotUser, gotTS, gotID = userID, throughCreatedAt, throughID
			return 7, nil
		},
	})
	n, err := h.Handle(context.Background(), in.MarkReadThroughCommand{Actor: actor, ThroughCreatedAt: ts, ThroughID: id})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n != 7 {
		t.Errorf("n = %d, want 7", n)
	}
	if gotUser != actor || !gotTS.Equal(ts) || gotID != id {
		t.Errorf("marker got (user=%v ts=%v id=%v)", gotUser, gotTS, gotID)
	}
}

func TestMarkReadThrough_ZeroCursorMeansAll(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	var gotTS time.Time
	var gotID uuid.UUID
	h := NewMarkReadThroughHandler(&fakeNotificationMarker{
		markReadThrough: func(_ uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
			gotTS, gotID = throughCreatedAt, throughID
			return 3, nil
		},
	})
	if _, err := h.Handle(context.Background(), in.MarkReadThroughCommand{Actor: actor}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !gotTS.IsZero() || gotID != uuid.Nil {
		t.Errorf("got (ts=%v id=%v), want zero/nil", gotTS, gotID)
	}
}
