// internal/message/application/command/post_system_event.go
package command

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

// PostSystemEventHandler implements the system-event-posting capability
// consumed by conversation's own SystemEventPoster port and call's own
// re-declared SystemEventPoster interface (both consumer-owned, per the
// design spec). It gets no port/in of its own — nothing inside this
// context's own HTTP controller ever calls it, so NewPostSystemEventHandler
// returns the concrete type directly rather than an interface.
type PostSystemEventHandler struct {
	writer    out.MessageWriter
	publisher out.MessagePublisher
	notif     out.NotificationProducer
}

// NewPostSystemEventHandler wires the PostSystemEvent capability.
func NewPostSystemEventHandler(writer out.MessageWriter, publisher out.MessagePublisher, notif out.NotificationProducer) *PostSystemEventHandler {
	return &PostSystemEventHandler{writer: writer, publisher: publisher, notif: notif}
}

// PostSystemEvent records a membership / lifecycle change as a system
// message in conv's timeline, so it gets a gap-free seq and fans out live as
// message.created. It is a no-op (returns nil) for direct conversations,
// and for channel_broadcast when the event is a member_* event (join/leave/
// kick churn there would flood the timeline). conversation_archived /
// conversation_unarchived are posted for every non-direct kind; call_started
// / call_ended are posted for every kind, direct included. A publish
// failure is logged; a persist failure is returned.
func (h *PostSystemEventHandler) PostSystemEvent(ctx context.Context, conv *domainconv.Conversation, actorID *uuid.UUID, event domainmsg.SystemEvent, targetID uuid.UUID, detail string) error {
	isCallEvent := event == domainmsg.SystemCallStarted || event == domainmsg.SystemCallEnded
	if conv.Kind == domainconv.KindDirect && !isCallEvent {
		return nil
	}
	isConversationEvent := event == domainmsg.SystemConversationArchived || event == domainmsg.SystemConversationUnarchived
	if !isConversationEvent && !isCallEvent && conv.Kind == domainconv.KindChannelBroadcast {
		return nil
	}
	m, err := domainmsg.NewSystemEvent(conv.ID, actorID, event, targetID, detail)
	if err != nil {
		return err
	}
	if err := h.writer.Create(ctx, m); err != nil {
		return fmt.Errorf("create system event: %w", err)
	}
	if err := h.publisher.PublishCreated(ctx, m); err != nil {
		log.Printf("message: publish system event %s: %v", m.ID, err)
	}
	switch event {
	case domainmsg.SystemMemberRemoved:
		if err := h.notif.MembershipEvent(ctx, event, conv.ID, targetID, ""); err != nil {
			log.Printf("message: notify membership-event %s: %v", conv.ID, err)
		}
	case domainmsg.SystemMemberTierChanged:
		if err := h.notif.MembershipEvent(ctx, event, conv.ID, targetID, detail); err != nil {
			log.Printf("message: notify membership-event %s: %v", conv.ID, err)
		}
	}
	return nil
}
