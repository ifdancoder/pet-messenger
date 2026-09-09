// Package pushtoken holds the PushToken aggregate: one registered device
// endpoint (web-push subscription JSON / FCM registration token / APNS device
// token) a user may receive device push notifications on.
package pushtoken

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Platform string

const (
	PlatformWeb  Platform = "web"
	PlatformFCM  Platform = "fcm"
	PlatformAPNS Platform = "apns"
)

func (p Platform) Valid() bool {
	switch p {
	case PlatformWeb, PlatformFCM, PlatformAPNS:
		return true
	default:
		return false
	}
}

const maxTokenLen = 4096

var (
	ErrInvalidUser     = errors.New("pushtoken: user must be a valid id")
	ErrInvalidPlatform = errors.New("pushtoken: unknown platform")
	ErrInvalidToken    = errors.New("pushtoken: token must be 1..4096 characters")
)

type PushToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Platform   Platform
	Token      string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

func New(userID uuid.UUID, platform Platform, token string) (*PushToken, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUser
	}
	if !platform.Valid() {
		return nil, ErrInvalidPlatform
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLen {
		return nil, ErrInvalidToken
	}
	now := time.Now().UTC()
	return &PushToken{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		Platform:   platform,
		Token:      token,
		CreatedAt:  now,
		LastUsedAt: now,
	}, nil
}

// Repository is the persistence port. Implemented in
// internal/infrastructure/postgres.
type Repository interface {
	// Upsert inserts t, or if (user_id, token) already exists refreshes
	// last_used_at. Returns the stored row.
	Upsert(ctx context.Context, t *PushToken) (*PushToken, error)
	// DeleteByToken removes one of the user's tokens; a missing row is not an
	// error.
	DeleteByToken(ctx context.Context, userID uuid.UUID, token string) error
	// DeleteToken removes a token regardless of owner — used by the notification
	// worker when a push provider reports the token is permanently gone.
	DeleteToken(ctx context.Context, token string) error
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*PushToken, error)
}
