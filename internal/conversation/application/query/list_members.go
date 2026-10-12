package query

import (
	"context"
	"fmt"

	"pet-messenger/internal/conversation/application"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

type listMembersHandler struct {
	conversations out.LoadConversationPort
	view          out.MemberViewPort
}

// NewListMembersHandler wires the ListMembers use case.
func NewListMembersHandler(conversations out.LoadConversationPort, view out.MemberViewPort) in.ListMembersUseCase {
	return &listMembersHandler{conversations: conversations, view: view}
}

// Handle returns a page of the conversation's roster. Any current member
// may read it. q.AdminsOnly restricts to tier-holders and the creator.
func (h *listMembersHandler) Handle(ctx context.Context, q in.ListMembersQuery) ([]*domain.MemberProfile, error) {
	if _, err := h.conversations.GetByID(ctx, q.ConversationID); err != nil {
		return nil, err // domain.ErrNotFound
	}
	if _, err := h.conversations.GetMember(ctx, q.ConversationID, q.Actor); err != nil {
		return nil, err // domain.ErrNotMember
	}
	list, err := h.view.ListMembers(ctx, q.ConversationID, q.AdminsOnly, q.Before, q.BeforeUserID, application.EffectiveListLimit(q.Limit))
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return list, nil
}
