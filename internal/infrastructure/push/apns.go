package push

import (
	"context"

	"pet-messenger/internal/domain/notification"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

// apnsStub stands in for a real APNS provider. Registering it keeps the
// PlatformAPNS route from silently disappearing; every Send fails loudly with
// ErrProviderNotConfigured until token-based APNS auth is implemented.
type apnsStub struct{}

func (apnsStub) Send(context.Context, *domainpush.PushToken, *notification.Notification) error {
	return ErrProviderNotConfigured
}
