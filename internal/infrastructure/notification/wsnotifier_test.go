package notification

import (
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
)

// startNATSConn returns the package-shared nats connection.
func startNATSConn(t *testing.T) *natsgo.Conn {
	t.Helper()
	return testNC
}

func TestWSNotifier_PublishesPerUserSubject(t *testing.T) {
	nc := startNATSConn(t)
	uid := uuid.Must(uuid.NewV7())

	got := make(chan *natsgo.Msg, 1)
	sub, err := nc.Subscribe("notif.ws."+uid.String(), func(m *natsgo.Msg) { got <- m })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	w := NewWSNotifier(nc)
	payload := []byte(`{"hello":"world"}`)
	if err := w.Notify(uid, payload); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case m := <-got:
		if string(m.Data) != string(payload) {
			t.Errorf("payload = %q, want %q", m.Data, payload)
		}
		if m.Subject != "notif.ws."+uid.String() {
			t.Errorf("subject = %q", m.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received on notif.ws.<uid>")
	}

	// A publish for a different user must not land on this subscription.
	if err := w.Notify(uuid.Must(uuid.NewV7()), []byte(`{}`)); err != nil {
		t.Fatalf("Notify other: %v", err)
	}
	select {
	case m := <-got:
		t.Fatalf("unexpected message for another user: %q on %s", m.Data, m.Subject)
	case <-time.After(300 * time.Millisecond):
	}
}
