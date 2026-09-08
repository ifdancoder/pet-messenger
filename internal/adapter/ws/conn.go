// Package ws is the WebSocket chat fan-out adapter: a per-pod Hub routes
// conversation payloads to locally-connected clients, fed by the NATS msg.>
// subscription (wired in internal/fx).
package ws

import (
	"sync"

	"github.com/google/uuid"
)

// Conn is one WebSocket client's server-side state: an outbound buffer drained
// by a single write-pump goroutine, plus a one-shot closed signal.
type Conn struct {
	userID uuid.UUID
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func newConn(userID uuid.UUID, buffer int) *Conn {
	return &Conn{
		userID: userID,
		out:    make(chan []byte, buffer),
		closed: make(chan struct{}),
	}
}

// enqueue hands payload to the write pump. If the buffer is full the client is
// too slow to keep up: the connection is closed (the client will reconnect and
// catch up from Postgres) rather than blocking the Hub.
func (c *Conn) enqueue(payload []byte) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.out <- payload:
	case <-c.closed:
	default:
		c.close()
	}
}

// close signals the pumps to shut the socket. Safe to call repeatedly.
func (c *Conn) close() {
	c.once.Do(func() { close(c.closed) })
}
