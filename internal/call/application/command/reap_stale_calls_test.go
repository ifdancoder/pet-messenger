package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

func TestReapStaleCalls(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{getErr: errors.New("node gone")}
	events := &fakeSystemEventPoster{}
	h := NewReapStaleCallsHandler(calls, calls, convs, nodes, &fakeRoomControl{}, events)
	conv1ID, user1ID := groupChat(t, convs)
	conv2ID := uuid.Must(uuid.NewV7())
	convs.extra = map[uuid.UUID]*conversation.Conversation{
		conv2ID: {ID: conv2ID, Kind: conversation.KindGroupChat},
	}

	c1 := mustCall(t, conv1ID, user1ID)
	c2 := mustCall(t, conv2ID, user1ID)
	calls.seedLive(c1)
	calls.seedLive(c2)

	n, err := h.Handle(context.Background(), in.ReapStaleCallsCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n != 2 {
		t.Fatalf("Handle = %d, want 2", n)
	}
	if calls.byID[c1.ID].IsLive() || calls.byID[c2.ID].IsLive() {
		t.Fatalf("both calls should be ended: %+v %+v", calls.byID[c1.ID], calls.byID[c2.ID])
	}
	var ended int
	for _, e := range events.events {
		if e == domainmsg.SystemCallEnded {
			ended++
		}
	}
	if ended != 2 {
		t.Fatalf("call_ended posted %d times, want 2", ended)
	}

	// Idempotent: nothing is live anymore, so a second sweep ends nothing.
	n, err = h.Handle(context.Background(), in.ReapStaleCallsCommand{})
	if err != nil {
		t.Fatalf("Handle (2nd): %v", err)
	}
	if n != 0 {
		t.Fatalf("Handle (2nd) = %d, want 0", n)
	}
}

func TestReapStaleCalls_MissingRoom(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	rooms := &fakeRoomControl{listParticipantsErr: out.ErrRoomNotFound}
	events := &fakeSystemEventPoster{}
	h := NewReapStaleCallsHandler(calls, calls, convs, &fakeMediaNodeRegistry{}, rooms, events)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)

	n, err := h.Handle(context.Background(), in.ReapStaleCallsCommand{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n != 1 || c.IsLive() {
		t.Fatalf("Handle = %d, call live = %v; want 1, false", n, c.IsLive())
	}
	if len(events.events) != 1 || events.events[0] != domainmsg.SystemCallEnded {
		t.Fatalf("events = %v, want one call_ended", events.events)
	}
}
