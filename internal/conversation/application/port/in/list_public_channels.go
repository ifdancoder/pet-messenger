package in

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type ListPublicChannelsQuery struct {
	Query    string
	Before   time.Time
	BeforeID uuid.UUID
	Limit    int
}

// ListPublicChannelsUseCase returns discoverable channels
// newest-activity-first.
type ListPublicChannelsUseCase interface {
	Handle(ctx context.Context, q ListPublicChannelsQuery) ([]*domain.Conversation, error)
}
