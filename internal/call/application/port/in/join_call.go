package in

import (
	"context"

	"github.com/google/uuid"

	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
)

// JoinCallCommand joins an already-live call by id.
type JoinCallCommand struct {
	Actor  uuid.UUID
	CallID uuid.UUID
	Role   domain.Role
}

// JoinCallUseCase handles JoinCallCommand.
type JoinCallUseCase interface {
	Handle(ctx context.Context, cmd JoinCallCommand) (out.JoinInfo, error)
}
