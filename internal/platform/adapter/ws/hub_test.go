package ws

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func drain(c *Conn) [][]byte {
	var got [][]byte
	for {
		select {
		case p := <-c.out:
			got = append(got, p)
		default:
			return got
		}
	}
}

func TestHub_SubscribeAndBroadcast(t *testing.T) {
	h := NewHub()
	cid := uuid.Must(uuid.NewV7())
	a, b := newConn(uuid.Must(uuid.NewV7()), 4), newConn(uuid.Must(uuid.NewV7()), 4)
	h.Subscribe(a, cid)
	h.Subscribe(b, cid)

	h.Broadcast(cid, []byte("hello"))

	for name, c := range map[string]*Conn{"a": a, "b": b} {
		got := drain(c)
		if len(got) != 1 || string(got[0]) != "hello" {
			t.Fatalf("%s got %v", name, got)
		}
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	cid := uuid.Must(uuid.NewV7())
	c := newConn(uuid.Must(uuid.NewV7()), 4)
	h.Subscribe(c, cid)
	h.Unsubscribe(c, cid)

	h.Broadcast(cid, []byte("x"))
	if got := drain(c); len(got) != 0 {
		t.Fatalf("got %v after unsubscribe", got)
	}
}

func TestHub_RemoveDropsEverything(t *testing.T) {
	h := NewHub()
	c1, c2 := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c := newConn(uuid.Must(uuid.NewV7()), 4)
	h.Subscribe(c, c1)
	h.Subscribe(c, c2)
	h.Remove(c)

	h.Broadcast(c1, []byte("x"))
	h.Broadcast(c2, []byte("y"))
	if got := drain(c); len(got) != 0 {
		t.Fatalf("got %v after Remove", got)
	}
	h.mu.RLock()
	n := len(h.subs)
	h.mu.RUnlock()
	if n != 0 {
		t.Fatalf("subs map not cleaned: %d entries", n)
	}
}

func TestHub_SlowConsumerDropped(t *testing.T) {
	h := NewHub()
	cid := uuid.Must(uuid.NewV7())
	c := newConn(uuid.Must(uuid.NewV7()), 1)
	h.Subscribe(c, cid)

	for i := 0; i < 5; i++ {
		h.Broadcast(cid, []byte("x")) // never drained -> buffer fills -> close
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("slow consumer was not closed")
	}
	// further broadcasts must not panic
	h.Broadcast(cid, []byte("x"))
}

func TestHub_BroadcastUnknownConversation(t *testing.T) {
	NewHub().Broadcast(uuid.Must(uuid.NewV7()), []byte("x")) // must not panic
}

func TestConn_EnqueueAfterClose(t *testing.T) {
	c := newConn(uuid.Must(uuid.NewV7()), 1)
	c.close()
	c.enqueue([]byte("x")) // no panic, no send
	if got := drain(c); len(got) != 0 {
		t.Fatalf("enqueue after close delivered %v", got)
	}
	c.close() // idempotent
}

func TestHub_DropSubscription(t *testing.T) {
	hub := NewHub()
	convA, convB := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	u1, u2 := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	c1a := newConn(u1, 4)
	c1b := newConn(u1, 4)
	c2 := newConn(u2, 4)
	hub.Subscribe(c1a, convA)
	hub.Subscribe(c1b, convA)
	hub.Subscribe(c2, convA)
	hub.Subscribe(c1a, convB)

	hub.DropSubscription(u1, convA)

	if hub.IsSubscribed(c1a, convA) || hub.IsSubscribed(c1b, convA) {
		t.Error("u1's conns should be gone from convA")
	}
	if !hub.IsSubscribed(c2, convA) {
		t.Error("u2's conn should remain in convA")
	}
	if !hub.IsSubscribed(c1a, convB) {
		t.Error("u1's convB subscription must be untouched")
	}

	// dropping the last member prunes the set (no panic, idempotent).
	hub.DropSubscription(u2, convA)
	hub.DropSubscription(u2, convA)
}

func TestHub_SendToUser(t *testing.T) {
	hub := NewHub()
	u1, u2 := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c1a := newConn(u1, 4)
	c1b := newConn(u1, 4)
	c2 := newConn(u2, 4)
	hub.Register(c1a)
	hub.Register(c1b)
	hub.Register(c2)

	if hub.UserConnCount(u1) != 2 || hub.UserConnCount(u2) != 1 {
		t.Fatalf("counts: u1=%d u2=%d", hub.UserConnCount(u1), hub.UserConnCount(u2))
	}

	hub.SendToUser(u1, []byte("hi"))
	for _, c := range []*Conn{c1a, c1b} {
		select {
		case got := <-c.out:
			if string(got) != "hi" {
				t.Fatalf("payload = %q", got)
			}
		default:
			t.Fatal("u1 conn did not receive")
		}
	}
	select {
	case <-c2.out:
		t.Fatal("u2 conn should not have received")
	default:
	}

	hub.Remove(c1a)
	if hub.UserConnCount(u1) != 1 {
		t.Fatalf("after Remove u1 count = %d, want 1", hub.UserConnCount(u1))
	}
	hub.Remove(c1b)
	if hub.UserConnCount(u1) != 0 {
		t.Fatalf("after Remove u1 count = %d, want 0", hub.UserConnCount(u1))
	}
}

func TestHub_ConcurrentAccess(t *testing.T) {
	h := NewHub()
	cid := uuid.Must(uuid.NewV7())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newConn(uuid.Must(uuid.NewV7()), 8)
			h.Subscribe(c, cid)
			h.Broadcast(cid, []byte("x"))
			h.Unsubscribe(c, cid)
			h.Remove(c)
		}()
	}
	wg.Wait()
}
