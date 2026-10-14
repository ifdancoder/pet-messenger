// internal/notification/application/port/out/notification_reader.go
package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/notification/domain"
)

// NotificationReader serves the notification-centre's two read paths.
// Implemented by internal/notification/adapter/out/persistence.NotificationRepository.
type NotificationReader interface {
	// ListForUser returns the user's notifications newest-first, keyset on
	// (created_at, id) DESC. A zero before starts from the newest; otherwise
	// only rows strictly older than the (before, beforeID) cursor are
	// returned. limit is a positive, already-clamped page size.
	ListForUser(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error)
	// CountUnread returns the user's unread-notification count for the badge.
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
}
