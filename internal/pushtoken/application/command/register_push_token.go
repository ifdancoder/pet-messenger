// internal/pushtoken/application/command/register_push_token.go
package command

import (
	"context"

	"pet-messenger/internal/pushtoken/application/port/in"
	"pet-messenger/internal/pushtoken/application/port/out"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

type registerPushTokenHandler struct {
	writer out.PushTokenWriter
}

// NewRegisterPushTokenHandler wires the RegisterPushToken use case.
func NewRegisterPushTokenHandler(writer out.PushTokenWriter) in.RegisterPushTokenUseCase {
	return &registerPushTokenHandler{writer: writer}
}

// Handle validates and stores the actor's push token, refreshing it if
// already registered.
func (h *registerPushTokenHandler) Handle(ctx context.Context, cmd in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
	tok, err := domainpush.New(cmd.Actor, cmd.Platform, cmd.Token)
	if err != nil {
		return nil, err
	}
	return h.writer.Upsert(ctx, tok)
}
