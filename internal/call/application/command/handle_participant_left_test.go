package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	domain "pet-messenger/internal/call/domain"
)

func TestHandleParticipantLeft(t *testing.T) {
	parts := newFakeParticipantStore()
	h := NewHandleParticipantLeftHandler(parts)
	callID := uuid.Must(uuid.NewV7())
	memberID := uuid.Must(uuid.NewV7())
	parts.parts[callID] = []*domain.Participant{{CallID: callID, UserID: memberID, JoinedAt: time.Now().UTC()}}

	if err := h.Handle(context.Background(), in.HandleParticipantLeftCommand{CallID: callID, UserID: memberID}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if parts.parts[callID][0].LeftAt == nil {
		t.Fatalf("participant row not marked left")
	}

	// Unknown call/user is not an error.
	if err := h.Handle(context.Background(), in.HandleParticipantLeftCommand{CallID: uuid.Must(uuid.NewV7()), UserID: uuid.Must(uuid.NewV7())}); err != nil {
		t.Fatalf("Handle (unknown): %v", err)
	}
}
