package medianode

import (
	"context"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	callout "pet-messenger/internal/call/application/port/out"
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

func TestRegistry_RegisterPickGet(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	r := NewRedisRegistry(rdb)
	r.ttl = 2 * time.Second

	if _, err := r.PickLeastLoaded(ctx); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("empty PickLeastLoaded: %v", err)
	}

	a := callout.Node{ID: "a", GRPCAddr: "a:1", WSAddr: "a:2"}
	b := callout.Node{ID: "b", GRPCAddr: "b:1", WSAddr: "b:2"}
	if err := r.Register(ctx, a, 50); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	if err := r.Register(ctx, b, 50); err != nil {
		t.Fatalf("Register b: %v", err)
	}
	// b is busier
	if err := r.Heartbeat(ctx, a, 50, Load{Rooms: 1, Participants: 2}); err != nil {
		t.Fatalf("hb a: %v", err)
	}
	if err := r.Heartbeat(ctx, b, 50, Load{Rooms: 3, Participants: 9}); err != nil {
		t.Fatalf("hb b: %v", err)
	}

	pick, err := r.PickLeastLoaded(ctx)
	if err != nil || pick.ID != "a" {
		t.Fatalf("PickLeastLoaded = %+v, %v (want a)", pick, err)
	}

	got, err := r.Get(ctx, "b")
	if err != nil || got.GRPCAddr != "b:1" {
		t.Fatalf("Get b = %+v, %v", got, err)
	}
	if _, err := r.Get(ctx, "missing"); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("Get missing: %v", err)
	}
}

func TestRegistry_SkipsFullAndExpired(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	r := NewRedisRegistry(rdb)
	r.ttl = 2 * time.Second

	full := callout.Node{ID: "full", GRPCAddr: "f:1"}
	if err := r.Register(ctx, full, 1); err != nil {
		t.Fatalf("Register full: %v", err)
	}
	if err := r.Heartbeat(ctx, full, 1, Load{Rooms: 1, Participants: 4}); err != nil {
		t.Fatalf("hb full: %v", err)
	}
	if _, err := r.PickLeastLoaded(ctx); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("a full node must be skipped: %v", err)
	}

	if err := r.Deregister(ctx, "full"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if _, err := r.Get(ctx, "full"); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("after Deregister: %v", err)
	}

	// TTL expiry
	exp := callout.Node{ID: "exp", GRPCAddr: "e:1"}
	_ = r.Register(ctx, exp, 50)
	time.Sleep(2500 * time.Millisecond)
	if _, err := r.Get(ctx, "exp"); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("expired node still present: %v", err)
	}
}
