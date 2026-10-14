// internal/notification/application/query/list_notifications.go

// Package query holds the CQRS read side of the notification context.
package query

import (
	"context"

	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/port/out"
	domainnotif "pet-messenger/internal/notification/domain"
)

// Page-size bounds for ListNotifications.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

// PageLimit clamps a client-requested page size: non-positive means "use
// the default"; anything above the max is capped.
func PageLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultPageLimit
	case requested > MaxPageLimit:
		return MaxPageLimit
	default:
		return requested
	}
}

type listNotificationsHandler struct {
	reader out.NotificationReader
}

// NewListNotificationsHandler wires the ListNotifications use case.
func NewListNotificationsHandler(reader out.NotificationReader) in.ListNotificationsUseCase {
	return &listNotificationsHandler{reader: reader}
}

// Handle returns a page of the actor's notifications, newest-first, keyset
// on (Before, BeforeID) DESC. A zero Before starts from the newest.
func (h *listNotificationsHandler) Handle(ctx context.Context, q in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
	return h.reader.ListForUser(ctx, q.Actor, q.Before, q.BeforeID, PageLimit(q.Limit))
}
