// Package in holds the user application layer's inbound ("driving")
// ports: one interface per use case, called by the one primary adapter
// that needs it (internal/user/adapter/in/web).
package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/user/domain"
)

// UpdateProfileCommand carries the optional profile fields to change. A
// nil field is left untouched. The web adapter rejects both-nil before
// calling Handle, matching today's HTTP contract exactly — Handle's own
// command.ErrNoFields guard is defense in depth for any future, non-HTTP
// caller, not the primary enforcement point.
type UpdateProfileCommand struct {
	UserID      uuid.UUID
	DisplayName *string
	Username    *string
}

// UpdateProfileUseCase applies a profile edit and returns the updated user.
type UpdateProfileUseCase interface {
	Handle(ctx context.Context, cmd UpdateProfileCommand) (*domain.User, error)
}
