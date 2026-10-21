package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

func newEndCallHandler(calls *fakeCallStore, convs *fakeConversationReader, nodes *fakeMediaNodeRegistry, rooms *fakeRoomControl, events *fakeSystemEventPoster) in.EndCallUseCase {
	return NewEndCallHandler(calls, calls, convs, nodes, rooms, events)
}

func TestEndCall_ByInitiator(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	rooms := &fakeRoomControl{}
	events := &fakeSystemEventPoster{}
	h := newEndCallHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, events)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)

	if err := h.Handle(context.Background(), in.EndCallCommand{Actor: initiatorID, CallID: c.ID}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rooms.closed) != 1 {
		t.Fatalf("rooms.closed = %d, want 1", len(rooms.closed))
	}
	if len(calls.endedCalls) != 1 {
		t.Fatalf("calls.endedCalls = %v, want the ended call", calls.endedCalls)
	}
	if c.IsLive() {
		t.Fatalf("call still live after Handle")
	}
	var sawEnded bool
	for _, e := range events.events {
		if e == domainmsg.SystemCallEnded {
			sawEnded = true
		}
	}
	if !sawEnded {
		t.Fatalf("events = %v, want a call_ended", events.events)
	}
}

func TestEndCall_ByAdminTierMember(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newEndCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, &fakeSystemEventPoster{})
	convID, _ := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)
	adminID := addMember(convs, convID, conversation.TierAdmin)

	if err := h.Handle(context.Background(), in.EndCallCommand{Actor: adminID, CallID: c.ID}); err != nil {
		t.Fatalf("Handle by admin: %v", err)
	}
	if c.IsLive() {
		t.Fatalf("call still live after Handle")
	}
}

func TestEndCall_ByPlainMemberForbidden(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newEndCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, &fakeSystemEventPoster{})
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)

	if err := h.Handle(context.Background(), in.EndCallCommand{Actor: memberID, CallID: c.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestEndCall_AlreadyEnded(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newEndCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, &fakeSystemEventPoster{})
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	_ = c.End()
	calls.byID[c.ID] = c

	if err := h.Handle(context.Background(), in.EndCallCommand{Actor: initiatorID, CallID: c.ID}); !errors.Is(err, domain.ErrAlreadyEnded) {
		t.Fatalf("err = %v, want ErrAlreadyEnded", err)
	}
}
