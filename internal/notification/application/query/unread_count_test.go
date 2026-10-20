// internal/notification/application/query/unread_count_test.go
package query

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/notification/application/port/in"
)

func TestUnreadCount_PassesThrough(t *testing.T) {
	reader := &fakeNotificationReader{countResult: 3}
	h := NewUnreadCountHandler(reader)
	actor := uuid.Must(uuid.NewV7())

	got, err := h.Handle(context.Background(), in.UnreadCountQuery{Actor: actor})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != 3 {
		t.Errorf("got = %d, want 3", got)
	}
}
