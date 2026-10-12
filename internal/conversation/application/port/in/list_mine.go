package in

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/out"
)

type ListMineQuery struct {
	UserID   uuid.UUID
	Before   time.Time
	BeforeID uuid.UUID
	Limit    int
}

// ListMineUseCase returns the caller's conversations newest-activity-first.
type ListMineUseCase interface {
	Handle(ctx context.Context, q ListMineQuery) ([]*out.InboxEntry, error)
}
