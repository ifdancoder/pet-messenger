package out

import (
	"context"

	"github.com/google/uuid"
)

// CallNotifier announces a new call to the conversation's other members
// (push/WS), independent of the system-timeline message SystemEventPoster
// writes.
type CallNotifier interface {
	CallStarted(ctx context.Context, callID, conversationID, initiatorID uuid.UUID) error
}
