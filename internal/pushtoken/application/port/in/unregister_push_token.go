// internal/pushtoken/application/port/in/unregister_push_token.go
package in

import (
	"context"

	"github.com/google/uuid"
)

// UnregisterPushTokenCommand is the input to UnregisterPushTokenUseCase.
type UnregisterPushTokenCommand struct {
	Actor uuid.UUID
	Token string
}

// UnregisterPushTokenUseCase removes one of the actor's device push
// endpoints. Removing an already-removed or unknown token is not an error.
type UnregisterPushTokenUseCase interface {
	Handle(ctx context.Context, cmd UnregisterPushTokenCommand) error
}
