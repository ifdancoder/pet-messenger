package command

import (
	"context"
	"errors"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

// ErrCannotKickSelf: RemoveMember was called with Actor == Target; use
// LeaveConversation instead.
var ErrCannotKickSelf = errors.New("conversation: cannot remove yourself; leave instead")

type removeMemberHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewRemoveMemberHandler wires the RemoveMember use case.
func NewRemoveMemberHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.RemoveMemberUseCase {
	return &removeMemberHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle kicks cmd.Target on cmd.Actor's authority. Rank rules live in the
// domain (domain.CanManageMember).
func (h *removeMemberHandler) Handle(ctx context.Context, cmd in.RemoveMemberCommand) error {
	if cmd.Actor == cmd.Target {
		return ErrCannotKickSelf
	}
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return err // domain.ErrNotFound
	}
	if conv.Kind == domain.KindDirect {
		return domain.ErrNotFound // a DM participant is never kickable
	}
	actorM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return err // domain.ErrNotMember
	}
	targetM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Target)
	if err != nil {
		return err // domain.ErrNotMember -> target absent
	}
	if !domain.CanManageMember(actorM, targetM) {
		return ErrForbidden
	}
	if err := h.writer.RemoveMember(ctx, cmd.ConversationID, cmd.Target); err != nil {
		return err
	}
	publishMemberRemoved(ctx, h.events, cmd.ConversationID, cmd.Target)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberRemoved, cmd.Target, ""); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return nil
}
