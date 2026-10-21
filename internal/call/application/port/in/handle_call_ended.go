package in

import (
	"context"

	"github.com/google/uuid"
)

// HandleCallEndedCommand finalizes callID, as reported by the
// media-service's call.<id>.ended NATS event (the room drained normally).
type HandleCallEndedCommand struct {
	CallID uuid.UUID
}

// HandleCallEndedUseCase handles HandleCallEndedCommand.
type HandleCallEndedUseCase interface {
	Handle(ctx context.Context, cmd HandleCallEndedCommand) error
}
