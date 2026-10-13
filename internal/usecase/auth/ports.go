// internal/usecase/auth/ports.go
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainuser "pet-messenger/internal/user/domain"
)

// PasswordHasher is implemented by internal/infrastructure/auth.BcryptHasher.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
}

// TokenIssuer is implemented by internal/infrastructure/auth.JWTIssuer.
type TokenIssuer interface {
	IssueAccessToken(userID uuid.UUID) (token string, expiresAt time.Time, err error)
}

// RefreshTokenGenerator is implemented by internal/infrastructure/auth.RefreshTokenGenerator.
type RefreshTokenGenerator interface {
	Generate() (plain string, hash string, err error)
	Hash(plain string) string
}

// RefreshTTL is the maximum lifetime of a refresh token since it was last
// issued or rotated, injected by internal/fx from config.Config.JWT.RefreshTTL.
type RefreshTTL time.Duration

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
