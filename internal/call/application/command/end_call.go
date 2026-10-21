package command

import (
	"context"
	"log"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
)

type endCallHandler struct {
	calls  out.CallLoader
	writer out.CallWriter
	convs  out.ConversationReader
	nodes  out.MediaNodeRegistry
	rooms  out.RoomControl
	events out.SystemEventPoster
}

// NewEndCallHandler wires the EndCall use case.
func NewEndCallHandler(calls out.CallLoader, writer out.CallWriter, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl, events out.SystemEventPoster) in.EndCallUseCase {
	return &endCallHandler{calls: calls, writer: writer, convs: convs, nodes: nodes, rooms: rooms, events: events}
}

// Handle ends cmd.CallID. Authorized for the call's initiator; for a
// direct call, also any conversation member; for a group/channel call,
// also any admin-tier member (see authorizeModeration).
func (h *endCallHandler) Handle(ctx context.Context, cmd in.EndCallCommand) error {
	c, err := h.calls.GetByID(ctx, cmd.CallID)
	if err != nil {
		return err
	}
	if !c.IsLive() {
		return domain.ErrAlreadyEnded
	}
	conv, err := h.convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		return err
	}
	if conv.Kind == conversation.KindDirect {
		if _, err := h.convs.GetMember(ctx, c.ConversationID, cmd.Actor); err != nil {
			return ErrForbidden
		}
	} else if err := authorizeModeration(ctx, h.convs, c, cmd.Actor); err != nil {
		return err
	}
	if node, gerr := h.nodes.Get(ctx, c.MediaNodeID); gerr == nil {
		if cerr := h.rooms.CloseRoom(ctx, node, cmd.CallID); cerr != nil {
			log.Printf("call: CloseRoom %s: %v", cmd.CallID, cerr)
		}
	}
	return finishCall(ctx, h.writer, h.convs, h.events, c, &cmd.Actor)
}
