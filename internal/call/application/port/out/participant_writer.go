package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// ParticipantWriter is the write half of call-participant persistence.
type ParticipantWriter interface {
	// AddParticipant inserts p. An existing active (call_id, user_id) row
	// returns domain.ErrAlreadyInCall and inserts nothing.
	AddParticipant(ctx context.Context, p *domain.Participant) error
	// MarkParticipantLeft stamps left_at on the user's active row. A
	// missing active row is not an error (idempotent).
	MarkParticipantLeft(ctx context.Context, callID, userID uuid.UUID, leftAt time.Time) error
}
