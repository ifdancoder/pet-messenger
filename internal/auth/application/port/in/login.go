package in

import "context"

// LoginCommand carries the parameters for LoginUseCase.
type LoginCommand struct {
	Username   string
	Password   string
	DeviceInfo string
}

// LoginUseCase verifies credentials and issues a new session.
type LoginUseCase interface {
	Handle(ctx context.Context, cmd LoginCommand) (*Result, error)
}
