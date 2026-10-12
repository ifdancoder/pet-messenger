// Package query holds the CQRS read side of the conversation context: one
// handler per query, each implementing the matching port/in use case.
package query

import (
	"context"
	"fmt"
	"strings"

	"pet-messenger/internal/conversation/application"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

type listPublicChannelsHandler struct {
	view out.PublicChannelViewPort
}

// NewListPublicChannelsHandler wires the ListPublicChannels use case.
func NewListPublicChannelsHandler(view out.PublicChannelViewPort) in.ListPublicChannelsUseCase {
	return &listPublicChannelsHandler{view: view}
}

// Handle returns discoverable channels newest-activity-first. q.Query is a
// case-insensitive title substring; q.Limit is clamped via
// application.EffectiveListLimit.
func (h *listPublicChannelsHandler) Handle(ctx context.Context, q in.ListPublicChannelsQuery) ([]*domain.Conversation, error) {
	list, err := h.view.ListPublicChannels(ctx, strings.TrimSpace(q.Query), q.Before, q.BeforeID, application.EffectiveListLimit(q.Limit))
	if err != nil {
		return nil, fmt.Errorf("list public channels: %w", err)
	}
	return list, nil
}
