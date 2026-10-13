package in

import (
	"context"

	"pet-messenger/internal/user/domain"
)

// GetByUsernameQuery carries the username to look up.
type GetByUsernameQuery struct {
	Username string
}

// GetByUsernameUseCase returns the public profile for a username.
type GetByUsernameUseCase interface {
	Handle(ctx context.Context, q GetByUsernameQuery) (*domain.User, error)
}
