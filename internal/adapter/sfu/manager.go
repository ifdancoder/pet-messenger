// Package sfu is the media-service selective forwarding unit. In this task it
// is pure bookkeeping — a set of rooms, each a set of participant ids — so the
// gRPC control-plane, the registry heartbeat, and cmd/media can be wired and
// tested before Pion is introduced (Task 11 adds real PeerConnections).
package sfu

import (
	"errors"
	"sync"

	"github.com/google/uuid"

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

type CallEventPublisher interface {
	ParticipantJoined(callID, userID uuid.UUID, role string)
	ParticipantLeft(callID, userID uuid.UUID)
	Ended(callID uuid.UUID)
}

type Manager interface {
	CreateRoom(callID uuid.UUID) error
	CloseRoom(callID uuid.UUID) error
	Mute(callID, userID uuid.UUID) error
	Remove(callID, userID uuid.UUID) error
	Stats(callID uuid.UUID) RoomStats
	Totals() Totals
}

type manager struct {
	cfg    *config.Config
	events CallEventPublisher
	mu     sync.Mutex
	rooms  map[uuid.UUID]*room
}

func NewManager(cfg *config.Config, events CallEventPublisher) Manager {
	return &manager{cfg: cfg, events: events, rooms: map[uuid.UUID]*room{}}
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
	m.rooms[callID] = newRoom(callID, m.cfg, m.events)
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

func (m *manager) withRoom(callID uuid.UUID, fn func(*room) error) error {
	m.mu.Lock()
	r, ok := m.rooms[callID]
	m.mu.Unlock()
	if !ok {
		return ErrRoomNotFound
	}
	return fn(r)
}

func (m *manager) Mute(callID, userID uuid.UUID) error {
	return m.withRoom(callID, func(r *room) error { return r.mute(userID) })
}

func (m *manager) Remove(callID, userID uuid.UUID) error {
	return m.withRoom(callID, func(r *room) error { return r.remove(userID) })
}

func (m *manager) Stats(callID uuid.UUID) RoomStats {
	m.mu.Lock()
	r, ok := m.rooms[callID]
	m.mu.Unlock()
	if !ok {
		return RoomStats{}
	}
	return RoomStats{Participants: r.count(), Exists: true}
}

func (m *manager) Totals() Totals {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := Totals{Rooms: len(m.rooms)}
	for _, r := range m.rooms {
		t.Participants += r.count()
	}
	return t
}

// roomDrained is called by a room when its last participant leaves.
func (m *manager) roomDrained(callID uuid.UUID) {
	m.mu.Lock()
	delete(m.rooms, callID)
	m.mu.Unlock()
	m.events.Ended(callID)
}
