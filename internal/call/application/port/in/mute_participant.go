package in

import (
	"context"

	"github.com/google/uuid"
)

// MuteParticipantCommand mutes targetID in callID, on actor's authority.
type MuteParticipantCommand struct {
	Actor    uuid.UUID
	CallID   uuid.UUID
	TargetID uuid.UUID
}

// MuteParticipantUseCase handles MuteParticipantCommand.
type MuteParticipantUseCase interface {
	Handle(ctx context.Context, cmd MuteParticipantCommand) error
}
