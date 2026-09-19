package sfu

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/config"
)

// noopEventPublisher is a CallEventPublisher that discards all events.
type noopEventPublisher struct{}

func (noopEventPublisher) ParticipantJoined(uuid.UUID, uuid.UUID, string) {}
func (noopEventPublisher) ParticipantLeft(uuid.UUID, uuid.UUID)           {}
func (noopEventPublisher) Ended(uuid.UUID)                                {}

// fakeSignaller discards everything sent to it — this test only cares about
// participant-count-driven timing, not the signalling traffic itself.
type fakeSignaller struct{}

func (fakeSignaller) Send([]byte) error { return nil }
func (fakeSignaller) Close()            {}

func TestRoomDrainsAfterSoloTimeout(t *testing.T) {
	api, err := newWebRTCAPI()
	if err != nil {
		t.Fatalf("newWebRTCAPI: %v", err)
	}
	drained := make(chan struct{}, 1)
	r := newRoom(uuid.Must(uuid.NewV7()), &config.Config{}, api, noopEventPublisher{}, func(*room) { drained <- struct{}{} }, time.Hour, 30*time.Millisecond)
	defer r.close()

	if err := r.join(uuid.Must(uuid.NewV7()), "publisher", fakeSignaller{}); err != nil {
		t.Fatalf("join: %v", err)
	}

	select {
	case <-drained:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("room did not drain after solo timeout")
	}
}

func TestRoomDoesNotDrainWithTwoParticipants(t *testing.T) {
	api, err := newWebRTCAPI()
	if err != nil {
		t.Fatalf("newWebRTCAPI: %v", err)
	}
	drained := make(chan struct{}, 1)
	r := newRoom(uuid.Must(uuid.NewV7()), &config.Config{}, api, noopEventPublisher{}, func(*room) { drained <- struct{}{} }, time.Hour, 30*time.Millisecond)
	defer r.close()

	if err := r.join(uuid.Must(uuid.NewV7()), "publisher", fakeSignaller{}); err != nil {
		t.Fatalf("join 1: %v", err)
	}
	if err := r.join(uuid.Must(uuid.NewV7()), "publisher", fakeSignaller{}); err != nil {
		t.Fatalf("join 2: %v", err)
	}

	select {
	case <-drained:
		t.Fatal("room drained even with two participants")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRoomViewerDoesNotKeepUnansweredCallAlive(t *testing.T) {
	api, err := newWebRTCAPI()
	if err != nil {
		t.Fatalf("newWebRTCAPI: %v", err)
	}
	drained := make(chan struct{}, 1)
	r := newRoom(uuid.Must(uuid.NewV7()), &config.Config{}, api, noopEventPublisher{}, func(*room) { drained <- struct{}{} }, time.Hour, 30*time.Millisecond)
	defer r.close()

	if err := r.join(uuid.Must(uuid.NewV7()), "publisher", fakeSignaller{}); err != nil {
		t.Fatalf("join publisher: %v", err)
	}
	if err := r.join(uuid.Must(uuid.NewV7()), "viewer", fakeSignaller{}); err != nil {
		t.Fatalf("join viewer: %v", err)
	}
	// The preview is not an accepted participant: it must not turn a lone
	// publisher's solo timeout into an indefinitely live room.
	select {
	case <-drained:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("viewer kept an otherwise unanswered call alive")
	}
}
