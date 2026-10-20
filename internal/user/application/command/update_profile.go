// Package command holds the CQRS write side of the user context: one
// handler per command, each implementing the matching port/in use case and
// depending only on the port/out interfaces it actually needs.
package command

import (
	"context"
	"errors"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/domain"
)

// ErrNoFields: UpdateProfile was called with both DisplayName and Username
// nil. The HTTP adapter already rejects this before calling Handle (see
// the design spec). This is defense in depth, not the primary enforcement.
var ErrNoFields = errors.New("user: no fields to update")

type updateProfileHandler struct {
	users out.UserStorePort
}

// NewUpdateProfileHandler wires the UpdateProfile use case.
func NewUpdateProfileHandler(users out.UserStorePort) in.UpdateProfileUseCase {
	return &updateProfileHandler{users: users}
}

// Handle loads the current user, applies whichever of DisplayName/Username
// is set, and persists both in one Update call: one business intent, one
// write, matching today's single-statement SQL UPDATE.
func (h *updateProfileHandler) Handle(ctx context.Context, cmd in.UpdateProfileCommand) (*domain.User, error) {
	if cmd.DisplayName == nil && cmd.Username == nil {
		return nil, ErrNoFields
	}
	u, err := h.users.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if cmd.DisplayName != nil {
		if err := u.SetDisplayName(*cmd.DisplayName); err != nil {
			return nil, err // domain.ErrInvalidDisplayName
		}
	}
	if cmd.Username != nil {
		if err := u.Rename(*cmd.Username); err != nil {
			return nil, err // domain.ErrInvalidUsername
		}
	}
	if err := h.users.Update(ctx, u); err != nil {
		return nil, err // domain.ErrUsernameTaken or domain.ErrNotFound
	}
	return u, nil
}
