package in

import (
	"context"

	"github.com/google/uuid"
)

// EndCallCommand ends a live call.
type EndCallCommand struct {
	Actor  uuid.UUID
	CallID uuid.UUID
}

// EndCallUseCase handles EndCallCommand.
type EndCallUseCase interface {
	Handle(ctx context.Context, cmd EndCallCommand) error
}
