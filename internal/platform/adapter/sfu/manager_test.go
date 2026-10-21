package sfu

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/platform/config"
)

// fakeEvents records every CallEventPublisher call for assertions.
type fakeEvents struct {
	mu     sync.Mutex
	joined []string
	left   []string
	ended  []uuid.UUID
}

func (f *fakeEvents) ParticipantJoined(callID, userID uuid.UUID, role string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joined = append(f.joined, callID.String()+"/"+userID.String()+"/"+role)
}

func (f *fakeEvents) ParticipantLeft(callID, userID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.left = append(f.left, callID.String()+"/"+userID.String())
}

func (f *fakeEvents) Ended(callID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended = append(f.ended, callID)
}

// sawEnded reports whether Ended was recorded for callID.
func (f *fakeEvents) sawEnded(callID uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.ended {
		if id == callID {
			return true
		}
	}
	return false
}

func newTestManager(t *testing.T, capacity int) (Manager, *fakeEvents) {
	t.Helper()
	ev := &fakeEvents{}
	cfg := &config.Config{Media: config.MediaConfig{RoomCapacity: capacity}}
	m, err := NewManager(cfg, ev)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, ev
}

func TestCreateRoomIdempotent(t *testing.T) {
	m, _ := newTestManager(t, 0)
	id := uuid.Must(uuid.NewV7())
	if err := m.CreateRoom(id); err != nil {
		t.Fatalf("first CreateRoom: %v", err)
	}
	if err := m.CreateRoom(id); err != nil {
		t.Fatalf("second CreateRoom: %v", err)
	}
	if got := m.Totals(); got.Rooms != 1 {
		t.Fatalf("Totals.Rooms = %d, want 1", got.Rooms)
	}
}

func TestCreateRoomAtCapacity(t *testing.T) {
	m, _ := newTestManager(t, 2)
	for i := 0; i < 2; i++ {
		if err := m.CreateRoom(uuid.Must(uuid.NewV7())); err != nil {
			t.Fatalf("CreateRoom %d: %v", i, err)
		}
	}
	if err := m.CreateRoom(uuid.Must(uuid.NewV7())); !errors.Is(err, ErrNodeAtCapacity) {
		t.Fatalf("CreateRoom past capacity = %v, want ErrNodeAtCapacity", err)
	}
}

func TestCloseRoomFiresEnded(t *testing.T) {
	m, ev := newTestManager(t, 0)
	id := uuid.Must(uuid.NewV7())
	if err := m.CreateRoom(id); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := m.CloseRoom(id); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}
	if len(ev.ended) != 1 || ev.ended[0] != id {
		t.Fatalf("ended = %v, want [%s]", ev.ended, id)
	}
	// Closing an unknown room is a no-op and fires no further event.
	if err := m.CloseRoom(uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("CloseRoom unknown: %v", err)
	}
	if len(ev.ended) != 1 {
		t.Fatalf("ended after no-op close = %v, want len 1", ev.ended)
	}
}

func TestStatsAndTotals(t *testing.T) {
	m, _ := newTestManager(t, 0)
	if st := m.Stats(uuid.Must(uuid.NewV7())); st.Exists || st.Participants != 0 {
		t.Fatalf("Stats of missing room = %+v, want zero value", st)
	}
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	if err := m.CreateRoom(a); err != nil {
		t.Fatalf("CreateRoom(a): %v", err)
	}
	if err := m.CreateRoom(b); err != nil {
		t.Fatalf("CreateRoom(b): %v", err)
	}
	if st := m.Stats(a); !st.Exists || st.Participants != 0 {
		t.Fatalf("Stats(a) = %+v, want {Exists:true Participants:0}", st)
	}
	if tot := m.Totals(); tot.Rooms != 2 || tot.Participants != 0 {
		t.Fatalf("Totals = %+v, want {Rooms:2 Participants:0}", tot)
	}
}

func TestListParticipants(t *testing.T) {
	m, _ := newTestManager(t, 0)
	callID := uuid.Must(uuid.NewV7())
	if err := m.CreateRoom(callID); err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, ok := m.ListParticipants(uuid.Must(uuid.NewV7())); ok {
		t.Fatal("expected ok=false for a call with no room")
	}
	userID := uuid.Must(uuid.NewV7())
	if err := m.Join(callID, userID, "publisher", nopSignaller{}); err != nil {
		t.Fatalf("join: %v", err)
	}
	peers, ok := m.ListParticipants(callID)
	if !ok {
		t.Fatal("expected ok=true for a live room")
	}
	if len(peers) != 1 || peers[0].UserID != userID.String() {
		t.Fatalf("unexpected participants: %+v", peers)
	}
}

func TestMuteRemoveMissingRoom(t *testing.T) {
	m, _ := newTestManager(t, 0)
	id := uuid.Must(uuid.NewV7())
	uid := uuid.Must(uuid.NewV7())
	if err := m.Mute(id, uid); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("Mute missing room = %v, want ErrRoomNotFound", err)
	}
	if err := m.Remove(id, uid); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("Remove missing room = %v, want ErrRoomNotFound", err)
	}
}

// nopSignaller is a Signaller that drops everything on the floor.
type nopSignaller struct{}

func (nopSignaller) Send([]byte) error { return nil }
func (nopSignaller) Close()            {}

func TestJoinAndClientFrameMissingRoom(t *testing.T) {
	m, _ := newTestManager(t, 0)
	id := uuid.Must(uuid.NewV7())
	uid := uuid.Must(uuid.NewV7())
	if err := m.Join(id, uid, "publisher", nopSignaller{}); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("Join missing room = %v, want ErrRoomNotFound", err)
	}
	// A frame for a room this node does not host is dropped, not a panic.
	m.ClientFrame(id, uid, []byte(`{"type":"candidate"}`))
}

func TestMuteRemoveMissingParticipant(t *testing.T) {
	m, _ := newTestManager(t, 0)
	id := uuid.Must(uuid.NewV7())
	if err := m.CreateRoom(id); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	uid := uuid.Must(uuid.NewV7())
	if err := m.Mute(id, uid); !errors.Is(err, ErrParticipantNotInRoom) {
		t.Fatalf("Mute missing participant = %v, want ErrParticipantNotInRoom", err)
	}
	if err := m.Remove(id, uid); !errors.Is(err, ErrParticipantNotInRoom) {
		t.Fatalf("Remove missing participant = %v, want ErrParticipantNotInRoom", err)
	}
}
