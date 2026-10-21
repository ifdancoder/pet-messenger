package out

import "context"

// Node is one media-service pod's addresses.
type Node struct {
	ID       string
	GRPCAddr string
	WSAddr   string
}

// MediaNodeRegistry is the read side of the Redis-backed media-node
// registry.
type MediaNodeRegistry interface {
	// PickLeastLoaded returns ErrNoMediaNodes when the registry is empty.
	PickLeastLoaded(ctx context.Context) (Node, error)
	Get(ctx context.Context, nodeID string) (Node, error)
}
