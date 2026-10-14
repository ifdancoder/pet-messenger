// internal/pushtoken/application/port/in/register_push_token.go

// Package in holds the pushtoken application layer's inbound ("driving")
// ports: one use-case interface per command, each with its own parameter
// struct.
package in

import (
	"context"

	"github.com/google/uuid"

	domainpush "pet-messenger/internal/pushtoken/domain"
)

// RegisterPushTokenCommand is the input to RegisterPushTokenUseCase.
type RegisterPushTokenCommand struct {
	Actor    uuid.UUID
	Platform domainpush.Platform
	Token    string
}

// RegisterPushTokenUseCase registers (or refreshes) one of the actor's
// device push endpoints.
type RegisterPushTokenUseCase interface {
	Handle(ctx context.Context, cmd RegisterPushTokenCommand) (*domainpush.PushToken, error)
}
