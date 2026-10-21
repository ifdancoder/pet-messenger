package ws

import (
	"sync"

	"github.com/google/uuid"
)

// Hub routes a conversation's payloads to the local connections subscribed to
// it. One Hub per pod; it holds no cross-pod state — the NATS subscription is
// the only inbound path and Broadcast the only fan-out.
type Hub struct {
	mu        sync.RWMutex
	subs      map[uuid.UUID]map[*Conn]struct{}
	userConns map[uuid.UUID]map[*Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		subs:      make(map[uuid.UUID]map[*Conn]struct{}),
		userConns: make(map[uuid.UUID]map[*Conn]struct{}),
	}
}

// Register adds c to its user's connection set. Called once per accepted
// WebSocket, before any subscribe. A Conn that never subscribes to a
// conversation still receives per-user notification pushes via SendToUser.
func (h *Hub) Register(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.userConns[c.userID]
	if set == nil {
		set = make(map[*Conn]struct{})
		h.userConns[c.userID] = set
	}
	set[c] = struct{}{}
}

// UserConnCount returns how many of userID's connections this pod holds.
func (h *Hub) UserConnCount(userID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.userConns[userID])
}

// SendToUser enqueues payload on every local connection belonging to userID.
// enqueue never blocks: a full buffer closes that connection.
func (h *Hub) SendToUser(userID uuid.UUID, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.userConns[userID] {
		c.enqueue(payload)
	}
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

// Remove drops c from every conversation it was subscribed to and from its
// user's connection set.
func (h *Hub) Remove(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for cid := range h.subs {
		h.removeLocked(c, cid)
	}
	if set := h.userConns[c.userID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.userConns, c.userID)
		}
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
// keeps receiving events after the user leaves the conversation until
// DropSubscription removes it (fed by the per-pod member.revoked.>
// subscription), or it unsubscribes or disconnects. A small residual race
// remains: an event already fanned out before the revoke propagates can still
// land on the departed member's socket.
func (h *Hub) Broadcast(conversationID uuid.UUID, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[conversationID] {
		c.enqueue(payload)
	}
}

// DropSubscription removes every Conn owned by userID from conversationID's
// fan-out set. The connections stay open with their other subscriptions intact;
// a later `subscribe` frame for this conversation is re-checked against
// GetMember and refused. Fed by the per-pod member.revoked.> subscription.
func (h *Hub) DropSubscription(userID, conversationID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[conversationID]
	for c := range set {
		if c.userID == userID {
			delete(set, c)
		}
	}
	if len(set) == 0 {
		delete(h.subs, conversationID)
	}
}
