package notify

import (
	"context"

	"github.com/google/uuid"

	domainpush "pet-messenger/internal/pushtoken/domain"
)

// Presence answers "does this user have a live WebSocket anywhere".
type Presence interface {
	Online(ctx context.Context, userID uuid.UUID) (bool, error)
}

// WSNotifier delivers a notification payload to a user's live WebSocket
// connections across the pod fleet.
type WSNotifier interface {
	Notify(userID uuid.UUID, payload []byte) error
}

// PushTokenLister is the slice of the pushtoken context's persistence
// adapter this worker needs: look up a user's registered devices before
// sending a push, and prune one a provider reports is permanently gone.
// Defined here (consumer-driven) per the project's retrofit design.
// Implemented by internal/pushtoken/adapter/out/persistence.PushTokenRepository.
type PushTokenLister interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*domainpush.PushToken, error)
	DeleteToken(ctx context.Context, token string) error
}
