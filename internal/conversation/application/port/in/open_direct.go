// Package in holds the conversation application layer's inbound
// ("driving") ports: one interface per use case, called by the one primary
// adapter that needs it (internal/conversation/adapter/in/web).
package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// OpenDirectCommand carries the parameters for OpenDirectUseCase.
type OpenDirectCommand struct {
	Me, Other uuid.UUID
}

// OpenDirectUseCase returns the direct conversation between Me and Other,
// creating it if it does not exist yet. Idempotent under a concurrent
// first-open race.
type OpenDirectUseCase interface {
	Handle(ctx context.Context, cmd OpenDirectCommand) (*domain.Conversation, error)
}
