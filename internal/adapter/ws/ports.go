package ws

import (
	"context"

	"github.com/google/uuid"
)

// PresenceRecorder records a user's live-connection state so the notification
// worker can decide between in-app delivery and device push. Implemented by
// internal/infrastructure/presence.RedisPresence.
type PresenceRecorder interface {
	Connected(ctx context.Context, userID uuid.UUID) error
	Heartbeat(ctx context.Context, userID uuid.UUID) error
	Disconnected(ctx context.Context, userID uuid.UUID) error
}

// NopPresence is a PresenceRecorder that does nothing — used by tests.
type NopPresence struct{}

func (NopPresence) Connected(context.Context, uuid.UUID) error    { return nil }
func (NopPresence) Heartbeat(context.Context, uuid.UUID) error    { return nil }
func (NopPresence) Disconnected(context.Context, uuid.UUID) error { return nil }
