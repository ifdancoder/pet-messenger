package in

import "context"

// LogoutCommand carries the parameters for LogoutUseCase.
type LogoutCommand struct {
	RefreshToken string
}

// LogoutUseCase revokes a session. Idempotent: an unknown refresh token is
// not an error.
type LogoutUseCase interface {
	Handle(ctx context.Context, cmd LogoutCommand) error
}
