package command

import (
	"context"
	"testing"

	in "pet-messenger/internal/call/application/port/in"
	domainmsg "pet-messenger/internal/message/domain"
)

func TestHandleCallEnded_Idempotent(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	events := &fakeSystemEventPoster{}
	h := NewHandleCallEndedHandler(calls, calls, convs, events)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)

	if err := h.Handle(context.Background(), in.HandleCallEndedCommand{CallID: c.ID}); err != nil {
		t.Fatalf("Handle #1: %v", err)
	}
	if err := h.Handle(context.Background(), in.HandleCallEndedCommand{CallID: c.ID}); err != nil {
		t.Fatalf("Handle #2: %v", err)
	}
	var ended int
	for _, e := range events.events {
		if e == domainmsg.SystemCallEnded {
			ended++
		}
	}
	if ended != 1 {
		t.Fatalf("call_ended posted %d times, want exactly 1", ended)
	}
}
