// internal/message/application/command/mark_read.go
package command

import (
	"context"

	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

type markReadHandler struct {
	loader        out.MessageLoader
	conversations out.ConversationMembershipPort
	cursor        out.ReadCursorPort
}

// NewMarkReadHandler wires the MarkRead use case.
func NewMarkReadHandler(loader out.MessageLoader, conversations out.ConversationMembershipPort, cursor out.ReadCursorPort) in.MarkReadUseCase {
	return &markReadHandler{loader: loader, conversations: conversations, cursor: cursor}
}

// Handle records that cmd.Actor has displayed cmd.MessageID and every
// earlier message in the conversation. Membership and message ownership are
// checked here rather than trusting a client-supplied cursor.
func (h *markReadHandler) Handle(ctx context.Context, cmd in.MarkReadCommand) error {
	if _, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor); err != nil {
		return err
	}
	m, err := h.loader.GetByID(ctx, cmd.MessageID)
	if err != nil {
		return err
	}
	if m.ConversationID != cmd.ConversationID {
		return domainmsg.ErrNotFound
	}
	return h.cursor.SetLastReadMessage(ctx, cmd.ConversationID, cmd.Actor, cmd.MessageID)
}
