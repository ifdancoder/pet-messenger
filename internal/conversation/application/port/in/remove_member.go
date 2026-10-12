package in

import (
	"context"

	"github.com/google/uuid"
)

type RemoveMemberCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	Target         uuid.UUID
}

// RemoveMemberUseCase kicks Target on Actor's authority.
type RemoveMemberUseCase interface {
	Handle(ctx context.Context, cmd RemoveMemberCommand) error
}
