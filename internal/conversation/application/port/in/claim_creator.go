package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type ClaimCreatorCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
}

// ClaimCreatorUseCase makes Actor the creator of a conversation whose seat
// is vacant.
type ClaimCreatorUseCase interface {
	Handle(ctx context.Context, cmd ClaimCreatorCommand) (*domain.Member, error)
}
