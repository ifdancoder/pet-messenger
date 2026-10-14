// internal/pushtoken/application/port/out/push_token_eraser.go
package out

import (
	"context"

	"github.com/google/uuid"
)

// PushTokenEraser removes one of a user's registered push tokens. Removing
// a token that does not exist is not an error. Implemented by
// internal/pushtoken/adapter/out/persistence.PushTokenRepository.
type PushTokenEraser interface {
	DeleteByToken(ctx context.Context, userID uuid.UUID, token string) error
}
