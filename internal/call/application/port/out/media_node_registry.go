package out

import "context"

// Node is one media-service pod's addresses, as a browser/api would reach
// them — not necessarily what the pod itself binds to (see
// config.MediaConfig.WSAddr's own doc comment). WSScheme is "ws" or "wss",
// kept apart from WSAddr rather than folded into one URL so WSAddr stays a
// plain host[:port] for every caller that only wants that.
type Node struct {
	ID       string
	GRPCAddr string
	WSAddr   string
	WSScheme string
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
