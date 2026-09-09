// Package presence tracks, in Redis, which users currently hold a live
// WebSocket connection on any api pod. presence:<userID> is a SET of pod ids
// with a TTL; a heartbeat refreshes the TTL, so a crashed pod's entry simply
// expires. Staleness is bounded by the TTL and only ever delays a push.
package presence

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const defaultTTL = 90 * time.Second

type RedisPresence struct {
	rdb   *goredis.Client
	podID string
	ttl   time.Duration
}

func NewRedisPresence(rdb *goredis.Client) *RedisPresence {
	pod, err := os.Hostname()
	if err != nil || pod == "" {
		pod = uuid.NewString()
	}
	return &RedisPresence{rdb: rdb, podID: pod, ttl: defaultTTL}
}

func key(userID uuid.UUID) string { return "presence:" + userID.String() }

// Connected records that this pod now serves at least one connection for
// userID.
func (p *RedisPresence) Connected(ctx context.Context, userID uuid.UUID) error {
	pipe := p.rdb.Pipeline()
	pipe.SAdd(ctx, key(userID), p.podID)
	pipe.Expire(ctx, key(userID), p.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("presence connected: %w", err)
	}
	return nil
}

// Heartbeat refreshes the TTL while a connection is alive.
func (p *RedisPresence) Heartbeat(ctx context.Context, userID uuid.UUID) error {
	if err := p.rdb.Expire(ctx, key(userID), p.ttl).Err(); err != nil {
		return fmt.Errorf("presence heartbeat: %w", err)
	}
	return nil
}

// Disconnected records that this pod no longer serves any connection for
// userID.
func (p *RedisPresence) Disconnected(ctx context.Context, userID uuid.UUID) error {
	if err := p.rdb.SRem(ctx, key(userID), p.podID).Err(); err != nil {
		return fmt.Errorf("presence disconnected: %w", err)
	}
	return nil
}

// Online reports whether userID has a live connection on any pod.
func (p *RedisPresence) Online(ctx context.Context, userID uuid.UUID) (bool, error) {
	n, err := p.rdb.Exists(ctx, key(userID)).Result()
	if err != nil {
		return false, fmt.Errorf("presence online: %w", err)
	}
	return n > 0, nil
}
