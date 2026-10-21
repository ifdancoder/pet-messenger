package command

import (
	"context"
	"errors"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
)

type handleCallEndedHandler struct {
	calls  out.CallLoader
	writer out.CallWriter
	convs  out.ConversationReader
	events out.SystemEventPoster
}

// NewHandleCallEndedHandler wires the HandleCallEnded use case.
func NewHandleCallEndedHandler(calls out.CallLoader, writer out.CallWriter, convs out.ConversationReader, events out.SystemEventPoster) in.HandleCallEndedUseCase {
	return &handleCallEndedHandler{calls: calls, writer: writer, convs: convs, events: events}
}

// Handle finalizes cmd.CallID. An already-gone call is not an error
// (idempotent against redelivery).
func (h *handleCallEndedHandler) Handle(ctx context.Context, cmd in.HandleCallEndedCommand) error {
	c, err := h.calls.GetByID(ctx, cmd.CallID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	return finishCall(ctx, h.writer, h.convs, h.events, c, nil)
}
