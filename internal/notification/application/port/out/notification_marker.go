// internal/notification/application/port/out/notification_marker.go
package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// NotificationMarker marks a user's notifications read. Implemented by
// internal/notification/adapter/out/persistence.NotificationRepository.
type NotificationMarker interface {
	// MarkReadThrough sets read_at = now() for every currently-unread
	// notification of the user with (created_at, id) <= (throughCreatedAt,
	// throughID). A zero throughCreatedAt means "everything". Returns the
	// count updated.
	MarkReadThrough(ctx context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error)
}
