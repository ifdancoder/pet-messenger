package in

import (
	"context"

	"github.com/google/uuid"
)

// UnmuteParticipantCommand unmutes targetID in callID, on actor's
// authority.
type UnmuteParticipantCommand struct {
	Actor    uuid.UUID
	CallID   uuid.UUID
	TargetID uuid.UUID
}

// UnmuteParticipantUseCase handles UnmuteParticipantCommand.
type UnmuteParticipantUseCase interface {
	Handle(ctx context.Context, cmd UnmuteParticipantCommand) error
}
