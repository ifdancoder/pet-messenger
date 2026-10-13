package in

import "context"

// RefreshCommand carries the parameters for RefreshUseCase.
type RefreshCommand struct {
	RefreshToken string
}

// RefreshUseCase rotates a refresh token and issues a new access token.
type RefreshUseCase interface {
	Handle(ctx context.Context, cmd RefreshCommand) (*Result, error)
}
