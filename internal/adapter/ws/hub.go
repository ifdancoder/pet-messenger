package ws

import (
	"sync"

	"github.com/google/uuid"
)

// Hub routes a conversation's payloads to the local connections subscribed to
// it. One Hub per pod; it holds no cross-pod state — the NATS subscription is
// the only inbound path and Broadcast the only fan-out.
type Hub struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[*Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uuid.UUID]map[*Conn]struct{})}
}

// Subscribe adds c to conversationID's fan-out set.
func (h *Hub) Subscribe(c *Conn, conversationID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[conversationID]
	if set == nil {
		set = make(map[*Conn]struct{})
		h.subs[conversationID] = set
	}
	set[c] = struct{}{}
}

// Unsubscribe removes c from conversationID's fan-out set.
func (h *Hub) Unsubscribe(c *Conn, conversationID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(c, conversationID)
}

// Remove drops c from every conversation it was subscribed to.
func (h *Hub) Remove(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for cid := range h.subs {
		h.removeLocked(c, cid)
	}
}

func (h *Hub) removeLocked(c *Conn, conversationID uuid.UUID) {
	set := h.subs[conversationID]
	if set == nil {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.subs, conversationID)
	}
}

// IsSubscribed reports whether c is in conversationID's fan-out set.
func (h *Hub) IsSubscribed(c *Conn, conversationID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.subs[conversationID][c]
	return ok
}

// Broadcast enqueues payload on every connection subscribed to conversationID.
// enqueue never blocks: a connection with a full buffer is closed by enqueue.
//
// Broadcast does not re-check membership: a Conn that subscribed while a member
// keeps receiving events after the user leaves the conversation, until it
// unsubscribes or disconnects. Closing that gap needs a membership-revocation
// path (deferred).
func (h *Hub) Broadcast(conversationID uuid.UUID, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[conversationID] {
		c.enqueue(payload)
	}
}
