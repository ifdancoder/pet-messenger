// internal/pushtoken/application/command/unregister_push_token.go
package command

import (
	"context"

	"pet-messenger/internal/pushtoken/application/port/in"
	"pet-messenger/internal/pushtoken/application/port/out"
)

type unregisterPushTokenHandler struct {
	eraser out.PushTokenEraser
}

// NewUnregisterPushTokenHandler wires the UnregisterPushToken use case.
func NewUnregisterPushTokenHandler(eraser out.PushTokenEraser) in.UnregisterPushTokenUseCase {
	return &unregisterPushTokenHandler{eraser: eraser}
}

// Handle removes one of the actor's push tokens. DeleteByToken treats a
// missing row as a no-op, so this is idempotent.
func (h *unregisterPushTokenHandler) Handle(ctx context.Context, cmd in.UnregisterPushTokenCommand) error {
	return h.eraser.DeleteByToken(ctx, cmd.Actor, cmd.Token)
}
