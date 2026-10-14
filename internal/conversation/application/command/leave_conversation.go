package command

import (
	"context"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

type leaveConversationHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewLeaveConversationHandler wires the LeaveConversation use case.
func NewLeaveConversationHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.LeaveConversationUseCase {
	return &leaveConversationHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle removes cmd.Actor's own membership from a group chat or channel
// and publishes a MemberRemoved event. A direct conversation cannot be left
// (refused as domain.ErrNotFound). The creator may leave; a successor is
// promoted in the same repository transaction if one remains.
func (h *leaveConversationHandler) Handle(ctx context.Context, cmd in.LeaveConversationCommand) error {
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return err // domain.ErrNotFound
	}
	if conv.Kind == domain.KindDirect {
		return domain.ErrNotFound
	}
	promoted, err := h.writer.RemoveMemberWithSuccession(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return err // domain.ErrNotMember
	}
	publishMemberRemoved(ctx, h.events, cmd.ConversationID, cmd.Actor)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberLeft, cmd.Actor, ""); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	if promoted != nil {
		publishMemberTierChanged(ctx, h.events, cmd.ConversationID, promoted.UserID, domain.TierCreatorAdmin)
		if err := h.systemEvents.PostSystemEvent(ctx, conv, nil, domainmsg.SystemMemberTierChanged, promoted.UserID, string(domain.TierCreatorAdmin)); err != nil {
			logPostSystemEventError(conv.ID, err)
		}
	}
	return nil
}
