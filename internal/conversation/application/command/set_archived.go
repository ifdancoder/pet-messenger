package command

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

type setArchivedHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewSetArchivedHandler wires the SetArchived use case.
func NewSetArchivedHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.SetArchivedUseCase {
	return &setArchivedHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle archives or unarchives a conversation on cmd.Actor's authority
// (superadmin+). Archiving makes it read-only and hides it from lists and
// discovery; unarchiving restores it. A no-op (already in the requested
// state) writes nothing and publishes nothing, after validating the actor.
func (h *setArchivedHandler) Handle(ctx context.Context, cmd in.SetArchivedCommand) error {
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return err // domain.ErrNotFound
	}
	actorM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return err // domain.ErrNotMember
	}
	if !domain.CanArchive(actorM) {
		return ErrForbidden
	}
	if conv.IsArchived() == cmd.Archived {
		return nil
	}
	if err := h.writer.SetArchived(ctx, cmd.ConversationID, cmd.Archived); err != nil {
		return err
	}

	systemEvent := domainmsg.SystemConversationArchived
	if cmd.Archived {
		if err := h.events.Publish(ctx, domain.ConversationArchived{ConversationID: cmd.ConversationID}); err != nil {
			logPublishError(cmd.ConversationID, err)
		}
	} else {
		systemEvent = domainmsg.SystemConversationUnarchived
		if err := h.events.Publish(ctx, domain.ConversationUnarchived{ConversationID: cmd.ConversationID}); err != nil {
			logPublishError(cmd.ConversationID, err)
		}
	}
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, systemEvent, uuid.Nil, ""); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return nil
}
