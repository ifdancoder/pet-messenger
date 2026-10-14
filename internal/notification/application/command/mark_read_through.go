// internal/notification/application/command/mark_read_through.go
package command

import (
	"context"

	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/port/out"
)

type markReadThroughHandler struct {
	marker out.NotificationMarker
}

// NewMarkReadThroughHandler wires the MarkReadThrough use case.
func NewMarkReadThroughHandler(marker out.NotificationMarker) in.MarkReadThroughUseCase {
	return &markReadThroughHandler{marker: marker}
}

// Handle delegates straight to the marker — no domain validation needed, a
// zero ThroughCreatedAt is a valid "mark everything" request.
func (h *markReadThroughHandler) Handle(ctx context.Context, cmd in.MarkReadThroughCommand) (int64, error) {
	return h.marker.MarkReadThrough(ctx, cmd.Actor, cmd.ThroughCreatedAt, cmd.ThroughID)
}
