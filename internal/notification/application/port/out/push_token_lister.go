// internal/notification/application/port/out/push_token_lister.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainpush "pet-messenger/internal/pushtoken/domain"
)

// PushTokenLister is the slice of the pushtoken context's persistence
// adapter this context needs: look up a user's registered devices before
// sending a push, and prune one a provider reports is permanently gone.
// Implemented by internal/pushtoken/adapter/out/persistence.PushTokenRepository.
type PushTokenLister interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*domainpush.PushToken, error)
	DeleteToken(ctx context.Context, token string) error
}
