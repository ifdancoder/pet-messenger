// internal/notification/application/port/out/pusher.go
package out

import (
	"context"

	domainnotif "pet-messenger/internal/notification/domain"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

// Pusher sends one device push notification. Implemented by
// internal/infrastructure/push.MultiPusher. An implementation returns an
// error satisfying errors.Is(err, push.ErrTokenGone) — from
// internal/infrastructure/push, deliberately not re-declared here, the same
// narrow shared-infra exception message's own retrofit used for
// internal/infrastructure/nats's wire schema — when the provider reports
// the token is permanently invalid.
type Pusher interface {
	Send(ctx context.Context, tok *domainpush.PushToken, n *domainnotif.Notification) error
}
