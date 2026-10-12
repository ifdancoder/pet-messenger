package in

import (
	"context"

	"github.com/google/uuid"
)

type LeaveConversationCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
}

// LeaveConversationUseCase removes Actor's own membership from a group
// chat or channel. A direct conversation cannot be left.
type LeaveConversationUseCase interface {
	Handle(ctx context.Context, cmd LeaveConversationCommand) error
}
