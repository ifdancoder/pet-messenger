// internal/notification/application/query/unread_count.go
package query

import (
	"context"

	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/port/out"
)

type unreadCountHandler struct {
	reader out.NotificationReader
}

// NewUnreadCountHandler wires the UnreadCount use case.
func NewUnreadCountHandler(reader out.NotificationReader) in.UnreadCountUseCase {
	return &unreadCountHandler{reader: reader}
}

// Handle returns the actor's unread-notification count.
func (h *unreadCountHandler) Handle(ctx context.Context, q in.UnreadCountQuery) (int, error) {
	return h.reader.CountUnread(ctx, q.Actor)
}
