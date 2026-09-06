// internal/domain/auth/session.go
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("session: not found")

type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	DeviceInfo       string
	CreatedAt        time.Time
	LastUsedAt       time.Time
	RevokedAt        *time.Time
}

func New(userID uuid.UUID, refreshTokenHash, deviceInfo string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:               uuid.New(),
		UserID:           userID,
		RefreshTokenHash: refreshTokenHash,
		DeviceInfo:       deviceInfo,
		CreatedAt:        now,
		LastUsedAt:       now,
	}
}

func (s *Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

// Repository stores refresh-token sessions. GetByRefreshTokenHash must return
// revoked sessions too (not filter them out) so callers can detect reuse of an
// already-rotated-away token.
type Repository interface {
	Create(ctx context.Context, s *Session) error
	GetByRefreshTokenHash(ctx context.Context, hash string) (*Session, error)
	// RotateSession atomically revokes oldSessionID and inserts newSession.
	RotateSession(ctx context.Context, oldSessionID uuid.UUID, newSession *Session) error
	Revoke(ctx context.Context, sessionID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}
