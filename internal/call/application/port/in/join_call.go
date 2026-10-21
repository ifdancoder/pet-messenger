package in

import (
	"context"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
	out "pet-messenger/internal/call/application/port/out"
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
