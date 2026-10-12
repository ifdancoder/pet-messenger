package command

import (
	"context"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

type claimCreatorHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewClaimCreatorHandler wires the ClaimCreator use case.
func NewClaimCreatorHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.ClaimCreatorUseCase {
	return &claimCreatorHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle makes cmd.Actor the creator of a conversation whose seat is
// vacant. A direct conversation is refused as domain.ErrNotFound (it is
// seat-less by construction). A non-member is refused as
// domain.ErrNotMember before any creator state is inspected. Only the
// highest-ranked current member may claim (domain.ErrNotCreatorHeir);
// domain.ErrCreatorSeatTaken if a creator already exists.
func (h *claimCreatorHandler) Handle(ctx context.Context, cmd in.ClaimCreatorCommand) (*domain.Member, error) {
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if conv.Kind == domain.KindDirect {
		return nil, domain.ErrNotFound
	}
	if _, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor); err != nil {
		return nil, err // domain.ErrNotMember
	}
	m, err := h.writer.ClaimCreator(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return nil, err // domain.ErrCreatorSeatTaken / ErrNotCreatorHeir / ErrNotMember
	}
	publishMemberTierChanged(ctx, h.events, cmd.ConversationID, cmd.Actor, domain.TierCreatorAdmin)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberTierChanged, cmd.Actor, string(domain.TierCreatorAdmin)); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return m, nil
}
