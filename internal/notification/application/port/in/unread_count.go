// internal/notification/application/port/in/unread_count.go
package in

import (
	"context"

	"github.com/google/uuid"
)

// UnreadCountQuery is the input to UnreadCountUseCase.
type UnreadCountQuery struct {
	Actor uuid.UUID
}

// UnreadCountUseCase returns the actor's unread-notification count for the
// badge.
type UnreadCountUseCase interface {
	Handle(ctx context.Context, q UnreadCountQuery) (int, error)
}
