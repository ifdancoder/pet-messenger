package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
	"pet-messenger/internal/domain/user"
)

// ErrForbidden: the actor is a member but their tier does not permit this
// action on this target. Shared by AddMember, RemoveMember, SetMemberTier.
var ErrForbidden = errors.New("conversation: actor is not permitted to perform this action")

// ErrInviteeNotFound: AddMember was asked to add a user id that does not
// resolve to an account.
var ErrInviteeNotFound = errors.New("conversation: invitee user does not exist")

type addMemberHandler struct {
	conversations out.LoadConversationPort
	writer        out.UpdateConversationPort
	users         user.Repository
	events        out.EventPublisherPort
	systemEvents  out.SystemEventPoster
}

// NewAddMemberHandler wires the AddMember use case.
func NewAddMemberHandler(conversations out.LoadConversationPort, writer out.UpdateConversationPort, users user.Repository, events out.EventPublisherPort, systemEvents out.SystemEventPoster) in.AddMemberUseCase {
	return &addMemberHandler{conversations: conversations, writer: writer, users: users, events: events, systemEvents: systemEvents}
}

// Handle adds cmd.Invitee to a group chat or channel on cmd.Actor's
// authority. Actor must hold an admin tier (rank >= 2) in a non-direct
// conversation. A channel_broadcast invitee joins as a subscriber;
// everywhere else as a plain member.
func (h *addMemberHandler) Handle(ctx context.Context, cmd in.AddMemberCommand) (*domain.Member, error) {
	conv, err := h.conversations.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err // domain.ErrNotFound
	}
	if conv.IsArchived() {
		return nil, domain.ErrConversationArchived
	}
	actorM, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Actor)
	if err != nil {
		return nil, err // domain.ErrNotMember
	}
	if !conv.CanAddMembers(actorM) {
		return nil, ErrForbidden
	}
	if _, err := h.users.GetByID(ctx, cmd.Invitee); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInviteeNotFound
		}
		return nil, fmt.Errorf("load invitee: %w", err)
	}
	role := domain.RoleMember
	if conv.Kind == domain.KindChannelBroadcast {
		role = domain.RoleSubscriber
	}
	m := &domain.Member{ConversationID: cmd.ConversationID, UserID: cmd.Invitee, Role: role, AdminTier: domain.TierNone, JoinedAt: time.Now().UTC()}
	if err := h.writer.AddMember(ctx, m); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}
	added, err := h.conversations.GetMember(ctx, cmd.ConversationID, cmd.Invitee)
	if err != nil {
		return nil, fmt.Errorf("reload added member: %w", err)
	}
	publishMemberAdded(ctx, h.events, conv, cmd.Invitee, nil)
	actor := cmd.Actor
	if err := h.systemEvents.PostSystemEvent(ctx, conv, &actor, domainmsg.SystemMemberJoined, cmd.Invitee, ""); err != nil {
		logPostSystemEventError(conv.ID, err)
	}
	return added, nil
}
