package in

import (
	"context"

	"github.com/google/uuid"
)

// RemoveParticipantCommand removes targetID from callID, on actor's
// authority.
type RemoveParticipantCommand struct {
	Actor    uuid.UUID
	CallID   uuid.UUID
	TargetID uuid.UUID
}

// RemoveParticipantUseCase handles RemoveParticipantCommand.
type RemoveParticipantUseCase interface {
	Handle(ctx context.Context, cmd RemoveParticipantCommand) error
}
