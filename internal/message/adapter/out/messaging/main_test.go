// internal/message/adapter/out/messaging/main_test.go
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

var testNC *natsgo.Conn

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

	code := m.Run()

	testNC.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
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
