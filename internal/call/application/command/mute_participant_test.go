package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	conversation "pet-messenger/internal/conversation/domain"
)

func newMuteParticipantHandler(calls *fakeCallStore, convs *fakeConversationReader, nodes *fakeMediaNodeRegistry, rooms *fakeRoomControl) in.MuteParticipantUseCase {
	return NewMuteParticipantHandler(calls, convs, nodes, rooms)
}

func TestMuteParticipant_ByInitiator(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	rooms := &fakeRoomControl{}
	h := newMuteParticipantHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms)
	convID, _ := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)
	target := addMember(convs, convID, conversation.TierNone)

	if err := h.Handle(context.Background(), in.MuteParticipantCommand{Actor: c.InitiatorID, CallID: c.ID, TargetID: target}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rooms.muted) != 1 {
		t.Fatalf("rooms.muted = %d, want 1", len(rooms.muted))
	}
}

func TestMuteParticipant_PlainMemberForbidden(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newMuteParticipantHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{})
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)
	target := addMember(convs, convID, conversation.TierNone)

	if err := h.Handle(context.Background(), in.MuteParticipantCommand{Actor: memberID, CallID: c.ID, TargetID: target}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestMuteParticipant_CannotTargetSelf(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newMuteParticipantHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{})
	convID, _ := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)

	if err := h.Handle(context.Background(), in.MuteParticipantCommand{Actor: c.InitiatorID, CallID: c.ID, TargetID: c.InitiatorID}); !errors.Is(err, ErrCannotTargetSelf) {
		t.Fatalf("err = %v, want ErrCannotTargetSelf", err)
	}
}
