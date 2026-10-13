// Package in holds the auth application layer's inbound ("driving")
// ports: one interface per use case, called by the one primary adapter
// that needs it (internal/auth/adapter/in/web).
package in

import (
	"context"
	"time"

	domainuser "pet-messenger/internal/user/domain"
)

// Result is returned by every use case that issues a session: Register,
// Login, and Refresh. Logout returns only an error.
type Result struct {
	User            *domainuser.User
	AccessToken     string
	AccessExpiresAt time.Time
	RefreshToken    string
}

// RegisterCommand carries the parameters for RegisterUseCase.
type RegisterCommand struct {
	Username    string
	Password    string
	DisplayName string
}

// RegisterUseCase creates a new account and immediately issues a session
// for it, exactly as LoginUseCase would.
type RegisterUseCase interface {
	Handle(ctx context.Context, cmd RegisterCommand) (*Result, error)
}
