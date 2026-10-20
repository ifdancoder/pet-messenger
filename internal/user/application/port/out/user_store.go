// Package out holds the user application layer's outbound ("driven")
// ports: what the use cases need from the outside world. Implemented by
// internal/user/adapter/out/persistence.UserRepository (UserStorePort) and
// internal/infrastructure/objectstore.MinioAvatarStore (AvatarStorePort).
package out

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/user/domain"
)

// UserStorePort is this context's own driven port for the write-side user
// record: everything the application layer's commands and queries need.
// It does NOT include Create or GetProfilesByIDs: no use case inside this
// package creates a user or looks up a batch of profiles (see this
// context's design spec, "Why Create (and GetProfilesByIDs) are not
// user's own ports"). Implemented by
// internal/user/adapter/out/persistence.UserRepository.
type UserStorePort interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	// Update persists u's current Username, DisplayName, and AvatarURL.
	// Returns domain.ErrUsernameTaken on a collision, domain.ErrNotFound
	// if u.ID doesn't exist.
	Update(ctx context.Context, u *domain.User) error
}
