// Package medianode is the Redis-backed registry of live media-service pods.
// Each pod SETs medianode:<id> (address + capacity + current load) with a TTL
// and refreshes it on a heartbeat; api-service reads the set to pick the
// least-loaded node for a new call and to resolve a call's assigned node.
package medianode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	callout "pet-messenger/internal/call/application/port/out"
)

const (
	keyPrefix  = "medianode:"
	defaultTTL = 15 * time.Second
)

// ErrNoMediaNodes means the registry has no node matching the request
// (none registered at all, or the specific id looked up is not present).
// The call command package recognizes any error from MediaNodeRegistry
// uniformly as "no media nodes" rather than matching this sentinel
// specifically, so it stays local to this adapter.
var ErrNoMediaNodes = errors.New("medianode: no media nodes available")

type Load struct {
	Rooms        int `json:"rooms"`
	Participants int `json:"participants"`
}

type entry struct {
	ID           string `json:"id"`
	GRPCAddr     string `json:"grpc_addr"`
	WSAddr       string `json:"ws_addr"`
	WSScheme     string `json:"ws_scheme"`
	Capacity     int    `json:"capacity"`
	Rooms        int    `json:"rooms"`
	Participants int    `json:"participants"`
	UpdatedAt    int64  `json:"updated_at"`
}

func (e entry) node() callout.Node {
	return callout.Node{ID: e.ID, GRPCAddr: e.GRPCAddr, WSAddr: e.WSAddr, WSScheme: e.WSScheme}
}

type RedisRegistry struct {
	rdb *goredis.Client
	ttl time.Duration
}

func NewRedisRegistry(rdb *goredis.Client) *RedisRegistry {
	return &RedisRegistry{rdb: rdb, ttl: defaultTTL}
}

func (r *RedisRegistry) put(ctx context.Context, e entry) error {
	e.UpdatedAt = time.Now().Unix()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal media node: %w", err)
	}
	if err := r.rdb.Set(ctx, keyPrefix+e.ID, b, r.ttl).Err(); err != nil {
		return fmt.Errorf("set media node: %w", err)
	}
	return nil
}

func (r *RedisRegistry) Register(ctx context.Context, self callout.Node, capacity int) error {
	return r.put(ctx, entry{ID: self.ID, GRPCAddr: self.GRPCAddr, WSAddr: self.WSAddr, WSScheme: self.WSScheme, Capacity: capacity})
}

func (r *RedisRegistry) Heartbeat(ctx context.Context, self callout.Node, capacity int, load Load) error {
	return r.put(ctx, entry{
		ID: self.ID, GRPCAddr: self.GRPCAddr, WSAddr: self.WSAddr, WSScheme: self.WSScheme, Capacity: capacity,
		Rooms: load.Rooms, Participants: load.Participants,
	})
}

func (r *RedisRegistry) Deregister(ctx context.Context, nodeID string) error {
	if err := r.rdb.Del(ctx, keyPrefix+nodeID).Err(); err != nil {
		return fmt.Errorf("del media node: %w", err)
	}
	return nil
}

func (r *RedisRegistry) Get(ctx context.Context, nodeID string) (callout.Node, error) {
	raw, err := r.rdb.Get(ctx, keyPrefix+nodeID).Bytes()
	if err == goredis.Nil {
		return callout.Node{}, ErrNoMediaNodes
	}
	if err != nil {
		return callout.Node{}, fmt.Errorf("get media node: %w", err)
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return callout.Node{}, fmt.Errorf("unmarshal media node: %w", err)
	}
	return e.node(), nil
}

func (r *RedisRegistry) PickLeastLoaded(ctx context.Context) (callout.Node, error) {
	var best *entry
	iter := r.rdb.Scan(ctx, 0, keyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		raw, err := r.rdb.Get(ctx, iter.Val()).Bytes()
		if err != nil {
			continue
		}
		var e entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		if e.Capacity > 0 && e.Rooms >= e.Capacity {
			continue
		}
		if best == nil ||
			e.Participants < best.Participants ||
			(e.Participants == best.Participants && e.Rooms < best.Rooms) ||
			(e.Participants == best.Participants && e.Rooms == best.Rooms && e.ID < best.ID) {
			cp := e
			best = &cp
		}
	}
	if err := iter.Err(); err != nil {
		return callout.Node{}, fmt.Errorf("scan media nodes: %w", err)
	}
	if best == nil {
		return callout.Node{}, ErrNoMediaNodes
	}
	return best.node(), nil
}

var (
	_ callout.MediaNodeRegistry = (*RedisRegistry)(nil)
)
