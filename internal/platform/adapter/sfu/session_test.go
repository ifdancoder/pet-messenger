package sfu

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestSessionReconnectPreservesRoomAndOwnership(t *testing.T) {
	for _, peerCount := range []int{1, 2} {
		t.Run(string(rune('0'+peerCount))+" peers", func(t *testing.T) {
			mgr, events := newTestManager(t, 0)
			callID, userID := uuid.New(), uuid.New()
			if err := mgr.CreateRoom(callID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = mgr.CloseRoom(callID) })
			stale, err := mgr.JoinSession(callID, userID, "publisher", nopSignaller{})
			if err != nil {
				t.Fatal(err)
			}
			if peerCount == 2 {
				// A publisher, not a viewer: this test is about reconnect and
				// moderator-removal bookkeeping, not about the (separately
				// tested, in room_test.go) rule that a viewer-only room
				// drains once the last publisher leaves. A viewer here would
				// make the moderator-removal assertion below trip that
				// unrelated rule instead of checking what it's meant to.
				if err := mgr.Join(callID, uuid.New(), "publisher", nopSignaller{}); err != nil {
					t.Fatal(err)
				}
			}
			if err := mgr.Mute(callID, userID); err != nil {
				t.Fatal(err)
			}
			current, err := mgr.JoinSession(callID, userID, "publisher", nopSignaller{})
			if err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			r := current.(*roomSession).room
			p := current.(*roomSession).participant
			if !ask(r, func() bool { return p.muted }) {
				t.Fatal("reconnect lifted moderator mute")
			}

			stale.ClientFrame([]byte(`{"type":"candidate","candidate":{"candidate":"old"}}`))
			stale.ClientFrame([]byte(`{"type":"bye"}`))
			stale.Close()
			if got := mgr.Stats(callID); !got.Exists || got.Participants != peerCount {
				t.Fatalf("stale session changed room: %+v", got)
			}
			if got := ask(r, func() int { return len(p.pendingCands) }); got != 0 {
				t.Fatalf("stale ICE candidate reached replacement: %d", got)
			}
			if events.sawEnded(callID) {
				t.Fatal("reconnect ended call")
			}
			events.mu.Lock()
			left := len(events.left)
			events.mu.Unlock()
			if left != 0 {
				t.Fatalf("reconnect emitted %d leave events", left)
			}

			current.ClientFrame([]byte(`{"type":"candidate","candidate":{"candidate":"current"}}`))
			if got := ask(r, func() int { return len(p.pendingCands) }); got != 1 {
				t.Fatalf("current session candidate count = %d", got)
			}
			if err := mgr.Remove(callID, userID); err != nil {
				t.Fatalf("moderator removal: %v", err)
			}
			if got := mgr.Stats(callID); got.Participants != peerCount-1 {
				t.Fatalf("moderator removal failed: %+v", got)
			}
		})
	}
}

func TestClosedRoomCommandsReturnError(t *testing.T) {
	mgr, _ := newTestManager(t, 0)
	callID, userID := uuid.New(), uuid.New()
	if err := mgr.CreateRoom(callID); err != nil {
		t.Fatal(err)
	}
	r, ok := mgr.(*manager).roomFor(callID)
	if !ok {
		t.Fatal("room missing")
	}
	if err := mgr.CloseRoom(callID); err != nil {
		t.Fatal(err)
	}
	<-r.done
	for name, invoke := range map[string]func() error{
		"join":   func() error { return r.join(userID, "publisher", nopSignaller{}) },
		"mute":   func() error { return r.mute(userID) },
		"remove": func() error { return r.remove(userID) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := invoke(); !errors.Is(err, ErrRoomNotFound) {
				t.Fatalf("closed room returned %v", err)
			}
		})
	}
}
