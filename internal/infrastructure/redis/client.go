// Package redis wraps the go-redis client construction with the project's fx
// lifecycle (Ping on start, Close on stop).
package redis

import (
	"context"
	"fmt"
	"log"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"pet-messenger/internal/config"
)

// NewClient dials cfg.Redis.URL, verifies it with a Ping, and closes it on
// fx stop. A failed dial fails app start — presence and the push gate depend
// on Redis.
func NewClient(lc fx.Lifecycle, cfg *config.Config) (*goredis.Client, error) {
	opt, err := goredis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := goredis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping %s: %w", cfg.Redis.URL, err)
	}
	log.Printf("redis: connected to %s", cfg.Redis.URL)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return rdb.Close() }})
	return rdb, nil
}
