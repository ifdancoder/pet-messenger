package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type CreateChannelCommand struct {
	Creator uuid.UUID
	Kind    domain.Kind
	Title   string
	Public  bool
}

// CreateChannelUseCase creates a channel_group or channel_broadcast owned
// by Creator.
type CreateChannelUseCase interface {
	Handle(ctx context.Context, cmd CreateChannelCommand) (*domain.Conversation, error)
}
