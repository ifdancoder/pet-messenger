// internal/pushtoken/application/port/out/push_token_writer.go

// Package out holds the pushtoken application layer's outbound ("driven")
// ports: what the use cases need from the outside world. Implemented by
// internal/pushtoken/adapter/out/persistence.
package out

import (
	"context"

	domainpush "pet-messenger/internal/pushtoken/domain"
)

// PushTokenWriter stores a validated push token, refreshing LastUsedAt if
// the (user, token) pair already exists. Implemented by
// internal/pushtoken/adapter/out/persistence.PushTokenRepository.
type PushTokenWriter interface {
	Upsert(ctx context.Context, t *domainpush.PushToken) (*domainpush.PushToken, error)
}
