package presence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newTestRedis(t *testing.T) *goredis.Client {
	t.Helper()
	ctx := context.Background()
	c, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	url, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	opt, _ := goredis.ParseURL(url)
	return goredis.NewClient(opt)
}

func TestRedisPresence(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	p := NewRedisPresence(rdb)
	p.podID = "pod-a"
	p.ttl = 2 * time.Second
	uid := uuid.Must(uuid.NewV7())

	online, err := p.Online(ctx, uid)
	if err != nil || online {
		t.Fatalf("fresh user should be offline: %v %v", online, err)
	}

	if err := p.Connected(ctx, uid); err != nil {
		t.Fatalf("Connected: %v", err)
	}
	if online, _ := p.Online(ctx, uid); !online {
		t.Fatal("should be online after Connected")
	}

	// a second pod keeps the user online after pod-a leaves
	p2 := NewRedisPresence(rdb)
	p2.podID = "pod-b"
	p2.ttl = 2 * time.Second
	if err := p2.Connected(ctx, uid); err != nil {
		t.Fatalf("p2 Connected: %v", err)
	}
	if err := p.Disconnected(ctx, uid); err != nil {
		t.Fatalf("Disconnected: %v", err)
	}
	if online, _ := p.Online(ctx, uid); !online {
		t.Fatal("pod-b still holds the user online")
	}
	if err := p2.Disconnected(ctx, uid); err != nil {
		t.Fatalf("p2 Disconnected: %v", err)
	}
	if online, _ := p.Online(ctx, uid); online {
		t.Fatal("both pods gone -> offline")
	}

	// TTL expiry with no heartbeat
	if err := p.Connected(ctx, uid); err != nil {
		t.Fatalf("Connected: %v", err)
	}
	time.Sleep(2500 * time.Millisecond)
	if online, _ := p.Online(ctx, uid); online {
		t.Fatal("un-refreshed presence key should have expired")
	}
}
