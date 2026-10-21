package command

import (
	"context"
	"errors"
	"log"
	"time"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

type startCallHandler struct {
	calls     out.CallWriter
	liveCalls out.CallLoader
	convs     out.ConversationReader
	nodes     out.MediaNodeRegistry
	rooms     out.RoomControl
	parts     out.ParticipantWriter
	tickets   out.JoinTicketIssuer
	ice       out.ICEConfigProvider
	events    out.SystemEventPoster
	notif     out.CallNotifier
}

// NewStartCallHandler wires the StartCall use case.
func NewStartCallHandler(calls out.CallWriter, liveCalls out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl, parts out.ParticipantWriter, tickets out.JoinTicketIssuer, ice out.ICEConfigProvider, events out.SystemEventPoster, notif out.CallNotifier) in.StartCallUseCase {
	return &startCallHandler{calls: calls, liveCalls: liveCalls, convs: convs, nodes: nodes, rooms: rooms, parts: parts, tickets: tickets, ice: ice, events: events, notif: notif}
}

// Handle starts a new call for cmd.ConversationID, or joins the
// conversation's already-live call if one exists (including the race where
// two actors start a call at the same instant: the loser's Create fails
// with domain.ErrLiveCallExists and it joins the winner's call instead).
func (h *startCallHandler) Handle(ctx context.Context, cmd in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
	conv, err := h.convs.GetByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, out.JoinInfo{}, err
	}
	if !callableKind(conv.Kind) {
		return nil, out.JoinInfo{}, ErrCallNotSupported
	}
	if conv.IsArchived() {
		return nil, out.JoinInfo{}, conversation.ErrConversationArchived
	}
	if _, err := h.convs.GetMember(ctx, cmd.ConversationID, cmd.Actor); err != nil {
		return nil, out.JoinInfo{}, err
	}

	if existing, err := h.liveCalls.GetLiveForConversation(ctx, cmd.ConversationID); err == nil {
		node, gerr := h.nodes.Get(ctx, existing.MediaNodeID)
		if gerr != nil {
			return nil, out.JoinInfo{}, ErrMediaNodeGone
		}
		if aerr := addParticipant(ctx, h.parts, existing.ID, cmd.Actor, domain.RolePublisher); aerr != nil {
			return nil, out.JoinInfo{}, aerr
		}
		ji, jerr := joinInfo(h.tickets, h.ice, existing.ID, cmd.Actor, domain.RolePublisher, node)
		return existing, ji, jerr
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, out.JoinInfo{}, err
	}

	node, err := h.nodes.PickLeastLoaded(ctx)
	if err != nil {
		return nil, out.JoinInfo{}, ErrNoMediaNodes
	}
	c, err := domain.New(cmd.ConversationID, cmd.Actor, node.ID)
	if err != nil {
		return nil, out.JoinInfo{}, err
	}
	if err := h.calls.Create(ctx, c); err != nil {
		if errors.Is(err, domain.ErrLiveCallExists) {
			won, rerr := h.liveCalls.GetLiveForConversation(ctx, cmd.ConversationID)
			if rerr != nil {
				return nil, out.JoinInfo{}, rerr
			}
			node2, gerr := h.nodes.Get(ctx, won.MediaNodeID)
			if gerr != nil {
				return nil, out.JoinInfo{}, ErrMediaNodeGone
			}
			_ = addParticipant(ctx, h.parts, won.ID, cmd.Actor, domain.RolePublisher)
			ji, jerr := joinInfo(h.tickets, h.ice, won.ID, cmd.Actor, domain.RolePublisher, node2)
			return won, ji, jerr
		}
		return nil, out.JoinInfo{}, err
	}

	if err := h.rooms.CreateRoom(ctx, node, c.ID); err != nil {
		if _, cerr := h.calls.MarkEnded(ctx, c.ID, time.Now().UTC()); cerr != nil {
			log.Printf("call: compensate MarkEnded %s: %v", c.ID, cerr)
		}
		return nil, out.JoinInfo{}, ErrMediaUnavailable
	}

	if err := addParticipant(ctx, h.parts, c.ID, cmd.Actor, domain.RolePublisher); err != nil {
		return nil, out.JoinInfo{}, err
	}
	if err := h.events.PostSystemEvent(ctx, conv, &cmd.Actor, domainmsg.SystemCallStarted, c.ID, ""); err != nil {
		log.Printf("call: post SystemCallStarted %s: %v", c.ID, err)
	}
	if err := h.notif.CallStarted(ctx, c.ID, cmd.ConversationID, cmd.Actor); err != nil {
		log.Printf("call: notify CallStarted %s: %v", c.ID, err)
	}

	ji, err := joinInfo(h.tickets, h.ice, c.ID, cmd.Actor, domain.RolePublisher, node)
	return c, ji, err
}
