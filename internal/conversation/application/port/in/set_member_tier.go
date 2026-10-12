package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type SetMemberTierCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	Target         uuid.UUID
	Tier           domain.AdminTier
}

// SetMemberTierUseCase promotes, demotes, or retiers Target on Actor's
// authority.
type SetMemberTierUseCase interface {
	Handle(ctx context.Context, cmd SetMemberTierCommand) (*domain.Member, error)
}
