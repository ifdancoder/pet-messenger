package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/fx/fxtest"

	"pet-messenger/internal/config"
)

func TestNewClientBadURL(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	cfg := &config.Config{Redis: config.RedisConfig{URL: "not-a-redis-url"}}
	if _, err := NewClient(lc, cfg); err == nil {
		t.Fatal("expected a parse error for a malformed REDIS_URL")
	}
}

func TestNewClientPingsAndClosesOnStop(t *testing.T) {
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

	lc := fxtest.NewLifecycle(t)
	rdb, err := NewClient(lc, &config.Config{Redis: config.RedisConfig{URL: url}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping after construct: %v", err)
	}

	lc.RequireStart()
	lc.RequireStop()

	if err := rdb.Ping(ctx).Err(); err != goredis.ErrClosed {
		t.Fatalf("client should be closed after fx stop, got %v", err)
	}
}
