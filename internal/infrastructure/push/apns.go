package push

import (
	"context"

	"pet-messenger/internal/domain/notification"
	"pet-messenger/internal/domain/pushtoken"
)

// apnsStub stands in for a real APNS provider. Registering it keeps the
// PlatformAPNS route from silently disappearing; every Send fails loudly with
// ErrProviderNotConfigured until token-based APNS auth is implemented.
type apnsStub struct{}

func (apnsStub) Send(context.Context, *pushtoken.PushToken, *notification.Notification) error {
	return ErrProviderNotConfigured
}
