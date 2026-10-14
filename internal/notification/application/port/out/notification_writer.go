// internal/notification/application/port/out/notification_writer.go

// Package out holds the notification application layer's outbound
// ("driven") ports: what the use cases need from the outside world.
// Implemented by internal/notification/adapter/out/persistence and (for the
// five relocated consumer-owned ports in this package) by other contexts'
// own adapters.
package out

import (
	"context"

	domainnotif "pet-messenger/internal/notification/domain"
)

// NotificationWriter persists a new Notification. Implemented by
// internal/notification/adapter/out/persistence.NotificationRepository.
type NotificationWriter interface {
	// Create inserts n. If n.DedupKey is non-empty and a row with the same
	// (UserID, DedupKey) exists, it returns domain.ErrDuplicate and inserts
	// nothing.
	Create(ctx context.Context, n *domainnotif.Notification) error
}
