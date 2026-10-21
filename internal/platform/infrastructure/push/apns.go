package push

import (
	"context"

	domainnotif "pet-messenger/internal/notification/domain"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

// apnsStub stands in for a real APNS provider. Registering it keeps the
// PlatformAPNS route from silently disappearing; every Send fails loudly with
// ErrProviderNotConfigured until token-based APNS auth is implemented.
type apnsStub struct{}

func (apnsStub) Send(context.Context, *domainpush.PushToken, *domainnotif.Notification) error {
	return ErrProviderNotConfigured
}
