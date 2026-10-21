// Package out holds the call context's outbound ("driven") ports: the
// narrow interfaces this context's own application layer calls, each
// satisfied by a secondary adapter.
package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// CallWriter is the write half of call persistence.
type CallWriter interface {
	// Create inserts c. A second live call for the same conversation
	// violates idx_calls_one_live_per_conversation; the caller (StartCall)
	// treats the resulting ErrLiveCallExists as "lost the race" and re-reads.
	Create(ctx context.Context, c *domain.Call) error
	// MarkEnded sets status='ended', ended_at=endedAt for a live call. The
	// bool reports whether a row actually transitioned (false when it was
	// already ended) so exactly one caller posts the timeline message.
	MarkEnded(ctx context.Context, id uuid.UUID, endedAt time.Time) (bool, error)
}
