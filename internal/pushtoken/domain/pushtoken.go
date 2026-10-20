// Package domain holds the PushToken aggregate: one registered device
// endpoint (web-push subscription JSON / FCM registration token / APNS
// device token) a user may receive device push notifications on. It is the
// Entities layer (Clean Architecture) / domain core (Hexagonal) for the
// pushtoken bounded context: no dependency on anything outside the
// standard library plus uuid.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Platform identifies which push provider a token belongs to.
type Platform string

// Supported push platforms.
const (
	PlatformWeb  Platform = "web"
	PlatformFCM  Platform = "fcm"
	PlatformAPNS Platform = "apns"
)

// Valid reports whether p is one of the known platforms.
func (p Platform) Valid() bool {
	switch p {
	case PlatformWeb, PlatformFCM, PlatformAPNS:
		return true
	default:
		return false
	}
}

const maxTokenLen = 4096

// Domain errors.
var (
	ErrInvalidUser     = errors.New("pushtoken: user must be a valid id")
	ErrInvalidPlatform = errors.New("pushtoken: unknown platform")
	ErrInvalidToken    = errors.New("pushtoken: token must be 1..4096 characters")
)

// PushToken is one registered device endpoint a user may receive push
// notifications on.
type PushToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Platform   Platform
	Token      string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// New validates and constructs a PushToken for userID on platform, trimming
// surrounding whitespace from token.
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
