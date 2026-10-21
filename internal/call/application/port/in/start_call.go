// Package in holds the call context's inbound ("driving") ports: one
// command or query struct, plus the one-method use-case interface that
// handles it, per operation. Each is implemented by exactly one handler in
// application/command or application/query.
package in

import (
	"context"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
	out "pet-messenger/internal/call/application/port/out"
)

// StartCallCommand starts a new call for a conversation, or joins the
// conversation's already-live call if one exists.
type StartCallCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
}

// StartCallUseCase handles StartCallCommand.
type StartCallUseCase interface {
	Handle(ctx context.Context, cmd StartCallCommand) (*domain.Call, out.JoinInfo, error)
}
