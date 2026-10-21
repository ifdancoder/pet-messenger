package command

import (
	"context"
	"log"
	"time"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
)

type joinCallHandler struct {
	calls   out.CallWriter
	loader  out.CallLoader
	convs   out.ConversationReader
	nodes   out.MediaNodeRegistry
	parts   out.ParticipantWriter
	tickets out.JoinTicketIssuer
	ice     out.ICEConfigProvider
}

// NewJoinCallHandler wires the JoinCall use case.
func NewJoinCallHandler(calls out.CallWriter, loader out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, parts out.ParticipantWriter, tickets out.JoinTicketIssuer, ice out.ICEConfigProvider) in.JoinCallUseCase {
	return &joinCallHandler{calls: calls, loader: loader, convs: convs, nodes: nodes, parts: parts, tickets: tickets, ice: ice}
}

// Handle joins cmd.Actor to the already-live call cmd.CallID. An invalid
// requested role silently falls back to RolePublisher, matching the
// pre-retrofit handler's behavior. Reconnecting (the actor already has an
// active participant row) is not an error.
func (h *joinCallHandler) Handle(ctx context.Context, cmd in.JoinCallCommand) (out.JoinInfo, error) {
	c, err := h.loader.GetByID(ctx, cmd.CallID)
	if err != nil {
		return out.JoinInfo{}, err
	}
	if !c.IsLive() {
		return out.JoinInfo{}, domain.ErrCallNotLive
	}
	if _, err := h.convs.GetMember(ctx, c.ConversationID, cmd.Actor); err != nil {
		return out.JoinInfo{}, err
	}
	role := cmd.Role
	if !role.Valid() {
		role = domain.RolePublisher
	}
	node, err := h.nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		if _, merr := h.calls.MarkEnded(ctx, c.ID, time.Now().UTC()); merr != nil {
			log.Printf("call: MarkEnded on gone node %s: %v", c.ID, merr)
		}
		return out.JoinInfo{}, ErrMediaNodeGone
	}
	if err := addParticipant(ctx, h.parts, c.ID, cmd.Actor, role); err != nil {
		return out.JoinInfo{}, err
	}
	return joinInfo(h.tickets, h.ice, c.ID, cmd.Actor, role, node)
}
