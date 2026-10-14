// internal/notification/application/port/out/presence.go
package out

import (
	"context"

	"github.com/google/uuid"
)

// Presence answers "does this user have a live WebSocket anywhere".
// Implemented by internal/infrastructure/presence.RedisPresence.
type Presence interface {
	Online(ctx context.Context, userID uuid.UUID) (bool, error)
}
