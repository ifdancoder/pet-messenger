package notify

import (
	"context"

	"github.com/google/uuid"
)

// Presence answers "does this user have a live WebSocket anywhere".
type Presence interface {
	Online(ctx context.Context, userID uuid.UUID) (bool, error)
}

// WSNotifier delivers a notification payload to a user's live WebSocket
// connections across the pod fleet.
type WSNotifier interface {
	Notify(userID uuid.UUID, payload []byte) error
}
