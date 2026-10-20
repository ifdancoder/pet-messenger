package command

import (
	"context"
	"errors"
	"fmt"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// ErrInvalidTier: SetMemberTier was given an unknown admin_tier value.
var ErrInvalidTier = errors.New("conversation: unknown admin tier")

type setMemberTierHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewSetMemberTierHandler wires the SetMemberTier use case.
func NewSetMemberTierHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.SetMemberTierUseCase {
	return &setMemberTierHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle promotes, demotes, or retiers cmd.Target on cmd.Actor's authority.
// It does not publish a MemberRemoved: a retiered member stays in the
// conversation. Who may assign what lives in the domain (CanAssignTier).
func (h *setMemberTierHandler) Handle(ctx context.Context, cmd in.SetMemberTierCommand) (*domain.Member, error) {
	if !cmd.Tier.Valid() {
		return nil, ErrInvalidTier
	}
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	actorM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return nil, err // domain.ErrNotMember
	}
	targetM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Target)
	if err != nil {
		return nil, err // domain.ErrNotMember
	}
	if !domain.CanAssignTier(actorM, targetM, cmd.Tier) {
		return nil, ErrForbidden
	}
	if err := h.writer.SetMemberTier(ctx, cmd.ConversationID, cmd.Target, cmd.Tier); err != nil {
		return nil, err
	}
	updated, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Target)
	if err != nil {
		return nil, fmt.Errorf("reload retiered member: %w", err)
	}
	publishMemberTierChanged(ctx, h.events, cmd.ConversationID, cmd.Target, cmd.Tier)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberTierChanged, cmd.Target, string(cmd.Tier)); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return updated, nil
}
