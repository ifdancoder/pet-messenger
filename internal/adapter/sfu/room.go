package sfu

import (
	"sync"

	"github.com/google/uuid"

	"pet-messenger/internal/config"
)

type room struct {
	callID uuid.UUID
	cfg    *config.Config
	events CallEventPublisher
	mu     sync.Mutex
	peers  map[uuid.UUID]*participant
	closed bool
}

func newRoom(callID uuid.UUID, cfg *config.Config, events CallEventPublisher) *room {
	return &room{callID: callID, cfg: cfg, events: events, peers: map[uuid.UUID]*participant{}}
}

func (r *room) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.peers)
}

func (r *room) mute(userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.peers[userID]
	if !ok {
		return ErrParticipantNotInRoom
	}
	p.muted = true
	return nil
}

func (r *room) remove(userID uuid.UUID) error {
	r.mu.Lock()
	p, ok := r.peers[userID]
	if ok {
		delete(r.peers, userID)
	}
	r.mu.Unlock()
	if !ok {
		return ErrParticipantNotInRoom
	}
	p.teardown()
	r.events.ParticipantLeft(r.callID, userID)
	return nil
}

func (r *room) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for id, p := range r.peers {
		p.teardown()
		delete(r.peers, id)
	}
}
