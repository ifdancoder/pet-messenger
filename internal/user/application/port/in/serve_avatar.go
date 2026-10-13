package in

import (
	"context"
	"io"
)

// ServeAvatarQuery carries the username whose avatar to serve.
type ServeAvatarQuery struct {
	Username string
}

// ServeAvatarUseCase resolves a username to its stored avatar bytes.
type ServeAvatarUseCase interface {
	Handle(ctx context.Context, q ServeAvatarQuery) (data io.ReadCloser, contentType string, err error)
}
