package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

type joinChannelHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewJoinChannelHandler wires the JoinChannel use case.
func NewJoinChannelHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.JoinChannelUseCase {
	return &joinChannelHandler{conversations: conversations, writer: writer, events: events, systemEvents: systemEvents}
}

// Handle adds cmd.Actor to a public channel and returns it. Idempotent: an
// actor who is already a member gets the channel back unchanged. A
// non-channel target, or a channel that is not public, is refused as
// domain.ErrNotFound so neither existence is disclosed; only once the
// target is known to be a public channel is an archived one reported as
// domain.ErrConversationArchived.
func (h *joinChannelHandler) Handle(ctx context.Context, cmd in.JoinChannelCommand) (*domain.Conversation, error) {
	conv, err := h.conversations.GetByID(ctx, cmd.ChannelID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if !conv.Kind.IsChannel() || !conv.Public {
		return nil, domain.ErrNotFound
	}
	if conv.IsArchived() {
		return nil, domain.ErrConversationArchived
	}
	if _, err := h.conversations.GetMember(ctx, cmd.ChannelID, cmd.Actor); err == nil {
		return conv, nil
	} else if !errors.Is(err, domain.ErrNotMember) {
		return nil, fmt.Errorf("check membership: %w", err)
	}

	role := domain.RoleMember
	if conv.Kind == domain.KindChannelBroadcast {
		role = domain.RoleSubscriber
	}
	m := &domain.Member{ConversationID: cmd.ChannelID, UserID: cmd.Actor, Role: role, JoinedAt: time.Now().UTC()}
	if err := h.writer.AddMember(ctx, m); err != nil {
		return nil, fmt.Errorf("add channel member: %w", err)
	}
	publishMemberAdded(ctx, h.events, conv, cmd.Actor, nil)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberJoined, cmd.Actor, ""); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return conv, nil
}
