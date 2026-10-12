package command

import (
	"context"
	"fmt"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

type createChannelHandler struct {
	writer out.UpdateConversationPort
	events out.EventPublisherPort
}

// NewCreateChannelHandler wires the CreateChannel use case.
func NewCreateChannelHandler(writer out.UpdateConversationPort, events out.EventPublisherPort) in.CreateChannelUseCase {
	return &createChannelHandler{writer: writer, events: events}
}

// Handle creates a channel_group or channel_broadcast owned by cmd.Creator.
// Kind/title validation lives in the domain constructor.
func (h *createChannelHandler) Handle(ctx context.Context, cmd in.CreateChannelCommand) (*domain.Conversation, error) {
	c, members, err := domain.NewChannel(cmd.Creator, cmd.Kind, cmd.Title, cmd.Public)
	if err != nil {
		return nil, err // domain.ErrInvalidCreator / ErrInvalidKind / ErrInvalidTitle
	}
	if err := h.writer.Create(ctx, c, members); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	publishConversationCreated(ctx, h.events, c, cmd.Creator)
	publishMemberAdded(ctx, h.events, c, cmd.Creator, nil)
	return c, nil
}
