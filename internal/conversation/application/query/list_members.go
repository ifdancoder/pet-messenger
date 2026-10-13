package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainuser "pet-messenger/internal/user/domain"
)

type listMembersHandler struct {
	conversations out.LoadConversationPort
	view          out.MemberViewPort
	profiles      out.ProfileBatchViewPort
}

// NewListMembersHandler wires the ListMembers use case.
func NewListMembersHandler(conversations out.LoadConversationPort, view out.MemberViewPort, profiles out.ProfileBatchViewPort) in.ListMembersUseCase {
	return &listMembersHandler{conversations: conversations, view: view, profiles: profiles}
}

// Handle returns a page of the conversation's roster, enriched with each
// member's public profile. Any current member may read it. q.AdminsOnly
// restricts to tier-holders and the creator.
func (h *listMembersHandler) Handle(ctx context.Context, q in.ListMembersQuery) ([]*domain.MemberProfile, error) {
	if _, err := h.conversations.GetByID(ctx, q.ConversationID); err != nil {
		return nil, err // domain.ErrNotFound
	}
	if _, err := h.conversations.GetMember(ctx, q.ConversationID, q.Actor); err != nil {
		return nil, err // domain.ErrNotMember
	}
	members, err := h.view.ListMembers(ctx, q.ConversationID, q.AdminsOnly, q.Before, q.BeforeUserID, application.EffectiveListLimit(q.Limit))
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	if len(members) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	profiles, err := h.profiles.GetProfilesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load member profiles: %w", err)
	}
	// GetProfilesByIDs returns no particular order (see its doc comment on
	// internal/user/adapter/out/persistence.UserRepository) — index by ID.
	byID := make(map[uuid.UUID]*domainuser.Profile, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}

	list := make([]*domain.MemberProfile, len(members))
	for i, m := range members {
		mp := &domain.MemberProfile{Member: *m}
		if p, ok := byID[m.UserID]; ok {
			mp.Username, mp.DisplayName, mp.AvatarURL = p.Username, p.DisplayName, p.AvatarURL
		}
		list[i] = mp
	}
	return list, nil
}
