package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type CreateGroupChatCommand struct {
	Creator uuid.UUID
	Title   string
}

// CreateGroupChatUseCase creates a group chat owned by Creator.
type CreateGroupChatUseCase interface {
	Handle(ctx context.Context, cmd CreateGroupChatCommand) (*domain.Conversation, error)
}
