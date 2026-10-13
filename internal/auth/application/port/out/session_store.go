package out

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/auth/domain"
)

// SessionStore stores refresh-token sessions. GetByRefreshTokenHash must
// return revoked sessions too (not filter them out) so callers can detect
// reuse of an already-rotated-away token. Implemented by
// internal/auth/adapter/out/persistence.SessionRepository.
type SessionStore interface {
	Create(ctx context.Context, s *domain.Session) error
	GetByRefreshTokenHash(ctx context.Context, hash string) (*domain.Session, error)
	// RotateSession atomically revokes oldSessionID and inserts newSession.
	RotateSession(ctx context.Context, oldSessionID uuid.UUID, newSession *domain.Session) error
	Revoke(ctx context.Context, sessionID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}
