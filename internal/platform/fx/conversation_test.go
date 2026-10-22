// internal/platform/fx/conversation_test.go
package fxapp

import (
	"context"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	"go.uber.org/fx"

	wsadapter "pet-messenger/internal/platform/adapter/ws"
	"pet-messenger/internal/conversation/adapter/out/messaging"
	"pet-messenger/internal/conversation/adapter/out/persistence"

	"github.com/google/uuid"
)

// noopInboxWriter satisfies messaging's unexported inboxWriter interface
// without a database: this test only proves the NATS subscriptions can be
// established together against a fresh stream, never that an event is
// actually projected.
type noopInboxWriter struct{}

func (noopInboxWriter) UpsertMember(context.Context, persistence.InboxRow) error { return nil }
func (noopInboxWriter) RemoveMember(context.Context, uuid.UUID, uuid.UUID, int64) error {
	return nil
}
func (noopInboxWriter) UpdateArchived(context.Context, uuid.UUID, *time.Time, int64) error {
	return nil
}
func (noopInboxWriter) UpdateLastMessageAt(context.Context, uuid.UUID, time.Time) error { return nil }

// TestConversationNATSWiring_FreshStream reproduces cmd/api's real startup
// sequence for the conversation module against a brand-new NATS JetStream
// instance (no pre-existing CONVERSATION_EVENTS stream or consumers): both
// fx.Invoke hooks the module registers, startInboxProjector's durable
// queue-group consumer and startMemberRevocationFanout's broadcast
// consumer, must be able to start together on the one stream, exactly as
// production wires them. Every other test and dev run in this project
// reuses an already-provisioned stream, which is what let a startup
// failure here go unnoticed.
func TestConversationNATSWiring_FreshStream(t *testing.T) {
	ctx := context.Background()

	nc, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("start nats container: %v", err)
	}
	t.Cleanup(func() { _ = nc.Terminate(ctx) })
	natsURL, err := nc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats connection string: %v", err)
	}

	// A freshly-booted nats server resets the first connections it accepts
	// even after the port is listening; retry briefly to absorb that window
	// (same pattern as the notification context's integration test).
	var conn *natsgo.Conn
	for range 20 {
		if conn, err = natsgo.Connect(natsURL); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(conn.Close)

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}

	projector := messaging.NewInboxProjector(noopInboxWriter{})
	hub := wsadapter.NewHub()

	app := fx.New(
		fx.Provide(
			func() *natsgo.Conn { return conn },
			func() natsgo.JetStreamContext { return js },
			func() *messaging.InboxProjector { return projector },
			func() *wsadapter.Hub { return hub },
		),
		fx.Invoke(startInboxProjector, startMemberRevocationFanout),
	)

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := app.Start(startCtx); err != nil {
		t.Fatalf("app failed to start against a fresh NATS instance: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Stop(stopCtx)
	})
}
