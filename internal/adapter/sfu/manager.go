// Package sfu is the media-service selective forwarding unit: one Pion
// PeerConnection per participant, one TrackLocalStaticRTP per published track
// fanned out to every other participant, and one actor goroutine per room that
// owns all of it.
package sfu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"

	"pet-messenger/internal/config"
)

var (
	ErrRoomNotFound   = errors.New("sfu: room not found")
	ErrRoomExists     = errors.New("sfu: room already exists")
	ErrNodeAtCapacity = errors.New("sfu: node at room capacity")
)

type RoomStats struct {
	Participants int
	Exists       bool
}

type Totals struct {
	Rooms        int
	Participants int
}

// Signaller is the room's view of one client's signalling channel. Task 12
// supplies a WebSocket implementation; tests supply an in-memory one.
type Signaller interface {
	// Send delivers one already-encoded frame to the client.
	Send(frame []byte) error
	// Close hangs the signalling channel up.
	Close()
}

type CallEventPublisher interface {
	ParticipantJoined(callID, userID uuid.UUID, role string)
	ParticipantLeft(callID, userID uuid.UUID)
	Ended(callID uuid.UUID)
}

type Manager interface {
	CreateRoom(callID uuid.UUID) error
	CloseRoom(callID uuid.UUID) error
	Join(callID, userID uuid.UUID, role string, sig Signaller) error
	ClientFrame(callID, userID uuid.UUID, frame []byte)
	Mute(callID, userID uuid.UUID) error
	Remove(callID, userID uuid.UUID) error
	Stats(callID uuid.UUID) RoomStats
	Totals() Totals
}

type manager struct {
	cfg    *config.Config
	api    *webrtc.API
	events CallEventPublisher
	mu     sync.Mutex
	rooms  map[uuid.UUID]*room
}

// NewManager builds the node's room registry around one shared webrtc.API.
func NewManager(cfg *config.Config, events CallEventPublisher) (Manager, error) {
	api, err := newWebRTCAPI()
	if err != nil {
		return nil, fmt.Errorf("sfu: build webrtc api: %w", err)
	}
	return &manager{cfg: cfg, api: api, events: events, rooms: map[uuid.UUID]*room{}}, nil
}

func (m *manager) CreateRoom(callID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rooms[callID]; ok {
		return nil // idempotent
	}
	if m.cfg.Media.RoomCapacity > 0 && len(m.rooms) >= m.cfg.Media.RoomCapacity {
		return ErrNodeAtCapacity
	}
	m.rooms[callID] = newRoom(callID, m.cfg, m.api, m.events, m.roomDrained)
	return nil
}

func (m *manager) CloseRoom(callID uuid.UUID) error {
	m.mu.Lock()
	r, ok := m.rooms[callID]
	if ok {
		delete(m.rooms, callID)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	r.close()
	m.events.Ended(callID)
	return nil
}

// roomFor looks a room up without holding the manager lock across any actor
// round-trip — a room draining itself takes that same lock.
func (m *manager) roomFor(callID uuid.UUID) (*room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[callID]
	return r, ok
}

func (m *manager) withRoom(callID uuid.UUID, fn func(*room) error) error {
	r, ok := m.roomFor(callID)
	if !ok {
		return ErrRoomNotFound
	}
	return fn(r)
}

func (m *manager) Join(callID, userID uuid.UUID, role string, sig Signaller) error {
	return m.withRoom(callID, func(r *room) error { return r.join(userID, role, sig) })
}

// ClientFrame routes one signalling frame from a client into the room actor.
func (m *manager) ClientFrame(callID, userID uuid.UUID, frame []byte) {
	if r, ok := m.roomFor(callID); ok {
		r.handleClientFrame(userID, frame)
	}
}

func (m *manager) Mute(callID, userID uuid.UUID) error {
	return m.withRoom(callID, func(r *room) error { return r.mute(userID) })
}

func (m *manager) Remove(callID, userID uuid.UUID) error {
	return m.withRoom(callID, func(r *room) error { return r.remove(userID) })
}

func (m *manager) Stats(callID uuid.UUID) RoomStats {
	r, ok := m.roomFor(callID)
	if !ok {
		return RoomStats{}
	}
	return RoomStats{Participants: r.count(), Exists: true}
}

func (m *manager) Totals() Totals {
	m.mu.Lock()
	rooms := make([]*room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.Unlock()

	t := Totals{Rooms: len(rooms)}
	for _, r := range rooms {
		t.Participants += r.count()
	}
	return t
}

// roomDrained is called by a room's actor when its last participant leaves or
// it idles out. A room the registry has already let go of (CloseRoom, or a
// replacement under the same call id) reports nothing — Ended fires once.
func (m *manager) roomDrained(r *room) {
	m.mu.Lock()
	cur, ok := m.rooms[r.callID]
	if ok = ok && cur == r; ok {
		delete(m.rooms, r.callID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	m.events.Ended(r.callID)
}
