// internal/conversation/adapter/out/messaging/main_test.go
package messaging

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

// testNC / testJS are a single JetStream-enabled nats container shared by
// every test in the package, mirroring
// internal/infrastructure/notification/main_test.go's rationale: one
// container plus a retrying connect avoids the nats startup race a fresh
// container per test would hit.
var (
	testNC *natsgo.Conn
	testJS natsgo.JetStreamContext
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		log.Fatalf("start nats container: %v", err)
	}
	url, err := container.ConnectionString(ctx)
	if err != nil {
		log.Fatalf("nats connection string: %v", err)
	}

	testNC, err = connectWithRetry(url)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	testJS, err = testNC.JetStream()
	if err != nil {
		log.Fatalf("jetstream context: %v", err)
	}
	if err := EnsureStream(testJS); err != nil {
		log.Fatalf("ensure %s stream: %v", streamName, err)
	}

	code := m.Run()

	testNC.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// purgeStream clears the CONVERSATION_EVENTS stream so each test starts
// from an empty backlog — without this, a WorkQueuePolicy stream shared
// across every test in this package would hand a new consumer whatever
// unclaimed message an earlier test left behind, not necessarily the one
// that test itself just published.
func purgeStream(t *testing.T) {
	t.Helper()
	if err := testJS.PurgeStream(streamName); err != nil {
		t.Fatalf("purge %s stream: %v", streamName, err)
	}
}

// connectWithRetry absorbs the brief window after the container reports
// ready where the nats server still resets incoming connections.
func connectWithRetry(url string) (*natsgo.Conn, error) {
	var lastErr error
	for range 20 {
		nc, err := natsgo.Connect(url)
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}
