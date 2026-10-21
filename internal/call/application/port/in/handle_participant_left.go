package in

import (
	"context"

	"github.com/google/uuid"
)

// HandleParticipantLeftCommand records that userID's media session for
// callID ended, as reported by the media-service's
// call.<id>.participant_left NATS event.
type HandleParticipantLeftCommand struct {
	CallID uuid.UUID
	UserID uuid.UUID
}

// HandleParticipantLeftUseCase handles HandleParticipantLeftCommand.
type HandleParticipantLeftUseCase interface {
	Handle(ctx context.Context, cmd HandleParticipantLeftCommand) error
}
