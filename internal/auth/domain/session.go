// internal/auth/domain/session.go

// Package domain holds the Session aggregate: a refresh-token session
// tied to one user and one device. It is the Entities layer (Clean
// Architecture) / domain core (Hexagonal) for the auth bounded context —
// no dependency on anything outside the standard library and uuid.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when no session matches the given lookup.
var ErrNotFound = errors.New("session: not found")

// Session is one refresh-token session for one device.
type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	DeviceInfo       string
	CreatedAt        time.Time
	LastUsedAt       time.Time
	RevokedAt        *time.Time
}

// New constructs a fresh, unrevoked Session.
func New(userID uuid.UUID, refreshTokenHash, deviceInfo string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:               uuid.Must(uuid.NewV7()),
		UserID:           userID,
		RefreshTokenHash: refreshTokenHash,
		DeviceInfo:       deviceInfo,
		CreatedAt:        now,
		LastUsedAt:       now,
	}
}

// IsRevoked reports whether this session has been revoked.
func (s *Session) IsRevoked() bool {
	return s.RevokedAt != nil
}
