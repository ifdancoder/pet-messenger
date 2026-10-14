// internal/message/application/command/delete_message.go
package command

import (
	"context"
	"fmt"
	"log"
	"time"

	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

type deleteMessageHandler struct {
	loader        out.MessageLoader
	writer        out.MessageWriter
	conversations out.ConversationMembershipPort
	publisher     out.MessagePublisher
}

// NewDeleteMessageHandler wires the DeleteMessage use case.
func NewDeleteMessageHandler(loader out.MessageLoader, writer out.MessageWriter, conversations out.ConversationMembershipPort, publisher out.MessagePublisher) in.DeleteMessageUseCase {
	return &deleteMessageHandler{loader: loader, writer: writer, conversations: conversations, publisher: publisher}
}

// Handle soft-deletes cmd.Actor's own message in cmd.ConversationID
// (idempotent). A non-author with a moderating admin tier may also delete.
// It returns the tombstoned message (Content cleared, DeletedAt set).
func (h *deleteMessageHandler) Handle(ctx context.Context, cmd in.DeleteMessageCommand) (*domainmsg.Message, error) {
	member, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return nil, err
	}
	m, err := h.loader.GetByID(ctx, cmd.MessageID)
	if err != nil {
		return nil, err
	}
	if m.ConversationID != cmd.ConversationID {
		return nil, domainmsg.ErrNotFound
	}
	if m.SenderID == nil || *m.SenderID != cmd.Actor {
		if !member.CanModerateMessages() {
			return nil, ErrNotAuthor
		}
	}
	m.MarkDeleted(time.Now().UTC())
	if err := h.writer.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	if err := h.publisher.PublishDeleted(ctx, m); err != nil {
		log.Printf("message: publish deleted %s: %v", m.ID, err)
	}
	return m, nil
}
