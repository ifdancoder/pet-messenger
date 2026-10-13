package out

import (
	"time"

	"github.com/google/uuid"
)

// TokenIssuer is implemented by internal/auth/adapter/out/jwt.Issuer.
type TokenIssuer interface {
	IssueAccessToken(userID uuid.UUID) (token string, expiresAt time.Time, err error)
}
