// Package notification holds the Notification aggregate: a per-user record of
// something worth telling the user about (a direct/group_chat message, a
// personal membership change), persisted so an unread badge and a
// notification centre survive the client being offline and so JetStream
// at-least-once redelivery is idempotent (DedupKey).
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindMessage           Kind = "message"
	KindMemberRemoved     Kind = "member_removed"
	KindMemberTierChanged Kind = "member_tier_changed"
)

func (k Kind) Valid() bool {
	switch k {
	case KindMessage, KindMemberRemoved, KindMemberTierChanged:
		return true
	default:
		return false
	}
}

var (
	ErrInvalidUser    = errors.New("notification: user must be a valid id")
	ErrInvalidKind    = errors.New("notification: unknown kind")
	ErrInvalidPayload = errors.New("notification: payload must be non-empty JSON")
	ErrNotFound       = errors.New("notification: not found")
	ErrDuplicate      = errors.New("notification: a notification with this dedup key already exists for the user")
)

type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Kind      Kind
	Payload   json.RawMessage
	DedupKey  string
	CreatedAt time.Time
	ReadAt    *time.Time
}

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

// Repository is the persistence port. Implemented in
// internal/infrastructure/postgres.
type Repository interface {
	// Create inserts n. If n.DedupKey is non-empty and a row with the same
	// (UserID, DedupKey) exists, it returns ErrDuplicate and inserts nothing.
	Create(ctx context.Context, n *Notification) error
	// ListForUser returns the user's notifications newest-first, keyset on
	// (created_at, id) DESC. A zero `before` starts from the newest; otherwise
	// only rows strictly older than the (before, beforeID) cursor are returned.
	// limit is a positive, already-clamped page size.
	ListForUser(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*Notification, error)
	// MarkReadThrough sets read_at = now() for every currently-unread
	// notification of the user with (created_at, id) <= (throughCreatedAt,
	// throughID). A zero throughCreatedAt means "everything". Returns the count
	// updated.
	MarkReadThrough(ctx context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
}
