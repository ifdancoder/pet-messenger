// internal/message/application/command/edit_message.go
package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

// ErrNotAuthor: edit/delete attempted by someone other than the sender (and,
// for delete, not a moderating admin tier either). Shared by EditMessage and
// DeleteMessage.
var ErrNotAuthor = errors.New("message: only the author may edit or delete this message")

type editMessageHandler struct {
	loader        out.MessageLoader
	writer        out.MessageWriter
	conversations out.ConversationMembershipPort
	publisher     out.MessagePublisher
}

// NewEditMessageHandler wires the EditMessage use case.
func NewEditMessageHandler(loader out.MessageLoader, writer out.MessageWriter, conversations out.ConversationMembershipPort, publisher out.MessagePublisher) in.EditMessageUseCase {
	return &editMessageHandler{loader: loader, writer: writer, conversations: conversations, publisher: publisher}
}

// Handle replaces the content of cmd.Actor's own text message in
// cmd.ConversationID. The message must belong to cmd.ConversationID and
// cmd.Actor must be both a current member and the original sender.
func (h *editMessageHandler) Handle(ctx context.Context, cmd in.EditMessageCommand) (*domainmsg.Message, error) {
	if _, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor); err != nil {
		return nil, err // domainconv.ErrNotMember
	}
	m, err := h.loader.GetByID(ctx, cmd.MessageID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if m.ConversationID != cmd.ConversationID {
		return nil, domainmsg.ErrNotFound
	}
	if m.SenderID == nil || *m.SenderID != cmd.Actor {
		return nil, ErrNotAuthor
	}
	if err := m.Edit(cmd.Content, time.Now().UTC()); err != nil {
		return nil, err // domain.ErrNotEditable / ErrEmptyContent / ErrContentTooLong
	}
	if err := h.writer.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
	if err := h.publisher.PublishUpdated(ctx, m); err != nil {
		log.Printf("message: publish updated %s: %v", m.ID, err)
	}
	return m, nil
}
