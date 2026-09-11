package call

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is the persistence port, implemented in
// internal/infrastructure/postgres.
type Repository interface {
	// Create inserts c. A second live call for the same conversation violates
	// idx_calls_one_live_per_conversation; the caller (StartCall) treats the
	// 23505 as "lost the race" and re-reads.
	Create(ctx context.Context, c *Call) error
	GetByID(ctx context.Context, id uuid.UUID) (*Call, error)
	// GetLiveForConversation returns the conversation's single active call, or
	// ErrNotFound when there is none.
	GetLiveForConversation(ctx context.Context, conversationID uuid.UUID) (*Call, error)
	// MarkEnded sets status='ended', ended_at=endedAt for a live call. The bool
	// reports whether a row actually transitioned (false when it was already
	// ended) so exactly one caller posts the timeline message.
	MarkEnded(ctx context.Context, id uuid.UUID, endedAt time.Time) (bool, error)
	// AddParticipant inserts p. An existing active (call_id, user_id) row →
	// ErrAlreadyInCall, insert nothing.
	AddParticipant(ctx context.Context, p *Participant) error
	// MarkParticipantLeft stamps left_at on the user's active row. A missing
	// active row is not an error (idempotent).
	MarkParticipantLeft(ctx context.Context, callID, userID uuid.UUID, leftAt time.Time) error
	ListParticipants(ctx context.Context, callID uuid.UUID) ([]*Participant, error)
	// ListLive returns every currently-active call, oldest first.
	ListLive(ctx context.Context) ([]*Call, error)
}
