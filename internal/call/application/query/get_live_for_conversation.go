// Package query holds the CQRS read side of the call context: currently
// one query, GetLiveForConversation.
package query

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

type getLiveForConversationHandler struct {
	convs  out.ConversationReader
	calls  out.CallLoader
	writer out.CallWriter
	nodes  out.MediaNodeRegistry
	rooms  out.RoomControl
	parts  out.ParticipantLister
	events out.SystemEventPoster
}

// NewGetLiveForConversationHandler wires the GetLiveForConversation use
// case.
func NewGetLiveForConversationHandler(convs out.ConversationReader, calls out.CallLoader, writer out.CallWriter, nodes out.MediaNodeRegistry, rooms out.RoomControl, parts out.ParticipantLister, events out.SystemEventPoster) in.GetLiveForConversationUseCase {
	return &getLiveForConversationHandler{convs: convs, calls: calls, writer: writer, nodes: nodes, rooms: rooms, parts: parts, events: events}
}

// Handle returns q.ConversationID's live call and its current participant
// roster. If the call's media node has no state for the room anymore
// (out.ErrRoomNotFound), Handle lazily finalizes the call -- a genuine
// write, even though this is otherwise a read-only query -- before
// returning domain.ErrNotFound, matching the pre-retrofit handler's
// behavior exactly. This is a deliberate, documented exception to CQRS
// purity, not an accident: see this package's own finishCall below.
func (h *getLiveForConversationHandler) Handle(ctx context.Context, q in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error) {
	if _, err := h.convs.GetMember(ctx, q.ConversationID, q.Actor); err != nil {
		return nil, nil, err
	}
	c, err := h.calls.GetLiveForConversation(ctx, q.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	node, err := h.nodes.Get(ctx, c.MediaNodeID)
	if err != nil {
		return c, []out.ParticipantInfo{}, nil
	}
	participants, err := h.rooms.ListParticipants(ctx, node, c.ID)
	if err != nil {
		if errors.Is(err, out.ErrRoomNotFound) {
			if ferr := h.finishCall(ctx, c); ferr != nil {
				return nil, nil, ferr
			}
			return nil, nil, domain.ErrNotFound
		}
		return c, []out.ParticipantInfo{}, nil
	}
	return c, h.withParticipationHistory(ctx, c.ID, participants), nil
}

// finishCall marks c ended and posts the call_ended timeline message,
// exactly once even if called twice for the same call.
//
// This is an INDEPENDENT COPY of the identically-named function in
// internal/call/application/command/shared.go, needed only for this
// query's own lazy-cleanup use above. Go gives no way to share unexported
// code across sibling packages without a third shared package, which this
// project has consistently declined to add (see e2ee's own
// callForMember/callKeyScope precedent). If you change this function,
// change the command package's copy too.
func (h *getLiveForConversationHandler) finishCall(ctx context.Context, c *domain.Call) error {
	transitioned, err := h.writer.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	conv, err := h.convs.GetByID(ctx, c.ConversationID)
	if err != nil {
		log.Printf("call: load conv for SystemCallEnded %s: %v", c.ID, err)
		return nil
	}
	if perr := h.events.PostSystemEvent(ctx, conv, nil, domainmsg.SystemCallEnded, c.ID, ""); perr != nil {
		log.Printf("call: post SystemCallEnded %s: %v", c.ID, perr)
	}
	return nil
}

// withParticipationHistory keeps the live roster useful for call UI after
// a participant leaves. A publisher who already accepted must not become
// a fresh ringing invitation merely because their current SFU session
// ended. Viewer rows are intentionally excluded: opening a docked preview
// is not accepting the call.
func (h *getLiveForConversationHandler) withParticipationHistory(ctx context.Context, callID uuid.UUID, live []out.ParticipantInfo) []out.ParticipantInfo {
	history, err := h.parts.ListParticipants(ctx, callID)
	if err != nil {
		log.Printf("call: list participation history %s: %v", callID, err)
		return live
	}
	accepted := make(map[string]bool, len(history))
	for _, participant := range history {
		if participant.Role == domain.RolePublisher {
			accepted[participant.UserID.String()] = true
		}
	}
	connected := make(map[string]bool, len(live))
	for i := range live {
		live[i].Connected = true
		live[i].Accepted = accepted[live[i].UserID]
		connected[live[i].UserID] = true
	}
	for userID := range accepted {
		if !connected[userID] {
			live = append(live, out.ParticipantInfo{UserID: userID, Role: string(domain.RolePublisher), TrackKinds: []string{}, Accepted: true})
		}
	}
	return live
}
