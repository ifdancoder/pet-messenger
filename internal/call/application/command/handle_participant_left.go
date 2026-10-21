package command

import (
	"context"
	"time"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
)

type handleParticipantLeftHandler struct {
	parts out.ParticipantWriter
}

// NewHandleParticipantLeftHandler wires the HandleParticipantLeft use
// case.
func NewHandleParticipantLeftHandler(parts out.ParticipantWriter) in.HandleParticipantLeftUseCase {
	return &handleParticipantLeftHandler{parts: parts}
}

// Handle stamps cmd.UserID's active participant row for cmd.CallID left.
// An unknown call/user pair is not an error (idempotent, matching
// ParticipantWriter.MarkParticipantLeft's own contract).
func (h *handleParticipantLeftHandler) Handle(ctx context.Context, cmd in.HandleParticipantLeftCommand) error {
	return h.parts.MarkParticipantLeft(ctx, cmd.CallID, cmd.UserID, time.Now().UTC())
}
