// internal/usecase/auth/ports.go
package auth

import (
	"time"

	"github.com/google/uuid"
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
