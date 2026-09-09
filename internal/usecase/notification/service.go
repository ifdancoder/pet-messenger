// Package notification is the usecase layer for the per-user notification
// centre: paging a user's notifications newest-first, marking them read through
// a keyset position, and reading the unread-badge count. It clamps the client
// page size and otherwise delegates to the notification.Repository port; it
// does not re-implement domain rules.
package notification

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/domain/notification"
)

// Page-size bounds for List.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

// Service is the notification usecase.
type Service struct {
	notifs domainnotif.Repository
}

// NewService wires the service. notifs is implemented by
// internal/infrastructure/postgres.NotificationRepository.
func NewService(notifs domainnotif.Repository) *Service {
	return &Service{notifs: notifs}
}

// PageLimit clamps a client-requested page size: non-positive means "use the
// default"; anything above the max is capped.
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

// List returns a page of the user's notifications, newest-first, keyset on
// (before, beforeID) DESC. A zero before starts from the newest. limit is
// clamped via PageLimit.
func (s *Service) List(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error) {
	return s.notifs.ListForUser(ctx, userID, before, beforeID, PageLimit(limit))
}

// MarkReadThrough marks every currently-unread notification of the user at or
// before the (throughCreatedAt, throughID) keyset position as read and returns
// the number updated. A zero throughCreatedAt means "all".
func (s *Service) MarkReadThrough(ctx context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
	return s.notifs.MarkReadThrough(ctx, userID, throughCreatedAt, throughID)
}

// UnreadCount returns the user's unread-notification count for the badge.
func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.notifs.CountUnread(ctx, userID)
}
