package command

import (
	"context"
	"fmt"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

type createGroupChatHandler struct {
	writer out.UpdateConversationPort
	events out.EventPublisherPort
}

// NewCreateGroupChatHandler wires the CreateGroupChat use case.
func NewCreateGroupChatHandler(writer out.UpdateConversationPort, events out.EventPublisherPort) in.CreateGroupChatUseCase {
	return &createGroupChatHandler{writer: writer, events: events}
}

// Handle creates a group chat owned by cmd.Creator. Title validation (trim,
// 1..256) lives in the domain constructor.
func (h *createGroupChatHandler) Handle(ctx context.Context, cmd in.CreateGroupChatCommand) (*domain.Conversation, error) {
	c, members, err := domain.NewGroupChat(cmd.Creator, cmd.Title)
	if err != nil {
		return nil, err // domain.ErrInvalidCreator / ErrInvalidTitle
	}
	if err := h.writer.Create(ctx, c, members); err != nil {
		return nil, fmt.Errorf("create group chat: %w", err)
	}
	publishConversationCreated(ctx, h.events, c, cmd.Creator)
	publishMemberAdded(ctx, h.events, c, cmd.Creator, nil)
	return c, nil
}
