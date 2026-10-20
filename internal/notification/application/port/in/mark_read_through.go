// internal/notification/application/port/in/mark_read_through.go
package in

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MarkReadThroughCommand is the input to MarkReadThroughUseCase.
type MarkReadThroughCommand struct {
	Actor            uuid.UUID
	ThroughCreatedAt time.Time
	ThroughID        uuid.UUID
}

// MarkReadThroughUseCase marks every currently-unread notification of the
// actor at or before the (ThroughCreatedAt, ThroughID) keyset position as
// read, and returns the number updated. A zero ThroughCreatedAt means
// "everything".
type MarkReadThroughUseCase interface {
	Handle(ctx context.Context, cmd MarkReadThroughCommand) (int64, error)
}
