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
	// PickLeastLoaded returns a non-nil error when the registry is empty.
	// Callers (StartCall) treat any error here as "no media nodes
	// available" rather than matching a specific sentinel, since this
	// port declares none: the concrete adapter is free to use its own
	// (see internal/infrastructure/medianode's own ErrNoMediaNodes).
	PickLeastLoaded(ctx context.Context) (Node, error)
	Get(ctx context.Context, nodeID string) (Node, error)
}
