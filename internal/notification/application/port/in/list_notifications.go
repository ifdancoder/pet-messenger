// internal/notification/application/port/in/list_notifications.go

// Package in holds the notification application layer's inbound
// ("driving") ports: one use-case interface per command/query, each with
// its own parameter struct.
package in

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/notification/domain"
)

// ListNotificationsQuery is the input to ListNotificationsUseCase.
type ListNotificationsQuery struct {
	Actor    uuid.UUID
	Before   time.Time
	BeforeID uuid.UUID
	Limit    int
}

// ListNotificationsUseCase returns a page of the actor's notifications,
// newest-first. Limit is clamped to [1, MaxPageLimit], defaulting to
// DefaultPageLimit when non-positive.
type ListNotificationsUseCase interface {
	Handle(ctx context.Context, q ListNotificationsQuery) ([]*domainnotif.Notification, error)
}
