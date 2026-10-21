package out

import (
	"context"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// ParticipantLister is the read half of call-participant persistence.
type ParticipantLister interface {
	ListParticipants(ctx context.Context, callID uuid.UUID) ([]*domain.Participant, error)
}
