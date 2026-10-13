// Package out holds the auth application layer's outbound ("driven")
// ports: what the use cases need from the outside world. Implemented by
// internal/auth/adapter/out/{persistence,bcrypt,refreshtoken,jwt} and
// internal/user/adapter/out/persistence.
package out

import (
	"context"

	"github.com/google/uuid"

	domainuser "pet-messenger/internal/user/domain"
)

// UserStore is this context's own out-port for everything it needs from
// the user bounded context: creating an account at registration, and
// loading one at login/refresh. Defined here (consumer-driven), not
// imported from the user context, per the project's retrofit design (see
// docs/superpowers/specs/2026-10-13-user-clean-hexagonal-cqrs-design.md).
// Implemented by internal/user/adapter/out/persistence.UserRepository.
type UserStore interface {
	Create(ctx context.Context, u *domainuser.User) error
	GetByUsername(ctx context.Context, username string) (*domainuser.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domainuser.User, error)
}
