// internal/notification/domain/notification.go

// Package domain holds the Notification aggregate: a per-user record of
// something worth telling the user about (a direct/group_chat message, a
// personal membership change, a call starting), persisted so an unread
// badge and a notification centre survive the client being offline and so
// JetStream at-least-once redelivery is idempotent (DedupKey). It is the
// Entities layer (Clean Architecture) / domain core (Hexagonal) for the
// notification bounded context.
package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Kind identifies what kind of event a Notification records.
type Kind string

// Notification kinds.
const (
	KindMessage           Kind = "message"
	KindMemberRemoved     Kind = "member_removed"
	KindMemberTierChanged Kind = "member_tier_changed"
	KindCallStarted       Kind = "call_started"
)

// Valid reports whether k is one of the known kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindMessage, KindMemberRemoved, KindMemberTierChanged, KindCallStarted:
		return true
	default:
		return false
	}
}

// Domain errors.
var (
	ErrInvalidUser    = errors.New("notification: user must be a valid id")
	ErrInvalidKind    = errors.New("notification: unknown kind")
	ErrInvalidPayload = errors.New("notification: payload must be non-empty JSON")
	ErrNotFound       = errors.New("notification: not found")
	ErrDuplicate      = errors.New("notification: a notification with this dedup key already exists for the user")
)

// Notification is a per-user record of something worth telling the user
// about.
type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Kind      Kind
	Payload   json.RawMessage
	DedupKey  string
	CreatedAt time.Time
	ReadAt    *time.Time
}

// New validates and constructs a Notification. dedupKey may be empty (no
// dedup enforced for that row).
func New(userID uuid.UUID, kind Kind, payload json.RawMessage, dedupKey string) (*Notification, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUser
	}
	if !kind.Valid() {
		return nil, ErrInvalidKind
	}
	if len(payload) == 0 || !json.Valid(payload) {
		return nil, ErrInvalidPayload
	}
	return &Notification{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Kind:      kind,
		Payload:   payload,
		DedupKey:  dedupKey,
		CreatedAt: time.Now().UTC(),
	}, nil
}
