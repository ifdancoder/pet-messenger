// Package query holds the CQRS read side of the user context: one handler
// per query, each implementing the matching port/in use case.
package query

import (
	"context"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/domain"
)

type getByUsernameHandler struct {
	users out.UserStorePort
}

// NewGetByUsernameHandler wires the GetByUsername use case.
func NewGetByUsernameHandler(users out.UserStorePort) in.GetByUsernameUseCase {
	return &getByUsernameHandler{users: users}
}

// Handle returns the public profile for q.Username.
func (h *getByUsernameHandler) Handle(ctx context.Context, q in.GetByUsernameQuery) (*domain.User, error) {
	return h.users.GetByUsername(ctx, q.Username) // domain.ErrNotFound on miss
}
