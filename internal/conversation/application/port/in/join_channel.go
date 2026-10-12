package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type JoinChannelCommand struct {
	Actor     uuid.UUID
	ChannelID uuid.UUID
}

// JoinChannelUseCase adds Actor to a public channel. Idempotent.
type JoinChannelUseCase interface {
	Handle(ctx context.Context, cmd JoinChannelCommand) (*domain.Conversation, error)
}
