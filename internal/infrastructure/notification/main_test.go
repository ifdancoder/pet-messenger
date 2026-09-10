package notification

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

// testNC / testJS are a single JetStream-enabled nats container shared by every
// test in the package. Spinning one container per test tripped the nats startup
// race (a fresh server resets the first connections it accepts -> "connect:
// EOF"); one container plus a retrying connect removes it and keeps the suite
// fast.
var (
	testNC *natsgo.Conn
	testJS natsgo.JetStreamContext
)

const notifStream = "NOTIF"

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
	if _, err := testJS.AddStream(&natsgo.StreamConfig{
		Name:      notifStream,
		Subjects:  []string{"notif.msg.*", "notif.member.*"},
		Retention: natsgo.WorkQueuePolicy,
		Storage:   natsgo.FileStorage,
		MaxAge:    24 * time.Hour,
		Discard:   natsgo.DiscardOld,
	}); err != nil {
		log.Fatalf("add NOTIF stream: %v", err)
	}

	code := m.Run()

	testNC.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// connectWithRetry absorbs the brief window after the container reports ready
// where the nats server still resets incoming connections.
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
