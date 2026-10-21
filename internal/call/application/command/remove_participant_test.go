package command

import (
	"context"
	"testing"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
)

func TestRemoveParticipant_ByAdmin(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	rooms := &fakeRoomControl{}
	parts := newFakeParticipantStore()
	h := NewRemoveParticipantHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, parts)
	convID, _ := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)
	adminID := addMember(convs, convID, conversation.TierAdmin)
	target := addMember(convs, convID, conversation.TierNone)
	parts.parts[c.ID] = []*domain.Participant{{CallID: c.ID, UserID: target}}

	if err := h.Handle(context.Background(), in.RemoveParticipantCommand{Actor: adminID, CallID: c.ID, TargetID: target}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rooms.removed) != 1 {
		t.Fatalf("rooms.removed = %d, want 1", len(rooms.removed))
	}
	if parts.parts[c.ID][0].LeftAt == nil {
		t.Fatalf("target participant row not marked left")
	}
}
