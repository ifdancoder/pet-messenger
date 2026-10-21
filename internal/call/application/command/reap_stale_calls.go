package command

import (
	"context"
	"errors"
	"log"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
)

type reapStaleCallsHandler struct {
	calls  out.CallLoader
	writer out.CallWriter
	convs  out.ConversationReader
	nodes  out.MediaNodeRegistry
	rooms  out.RoomControl
	events out.SystemEventPoster
}

// NewReapStaleCallsHandler wires the ReapStaleCalls use case.
func NewReapStaleCallsHandler(calls out.CallLoader, writer out.CallWriter, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl, events out.SystemEventPoster) in.ReapStaleCallsUseCase {
	return &reapStaleCallsHandler{calls: calls, writer: writer, convs: convs, nodes: nodes, rooms: rooms, events: events}
}

// Handle sweeps every live call and finalizes any whose media node is gone
// from the registry, or whose node no longer has the room, returning how
// many it ended.
func (h *reapStaleCallsHandler) Handle(ctx context.Context, cmd in.ReapStaleCallsCommand) (int, error) {
	live, err := h.calls.ListLive(ctx)
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, c := range live {
		node, gerr := h.nodes.Get(ctx, c.MediaNodeID)
		if gerr != nil {
			if err := finishCall(ctx, h.writer, h.convs, h.events, c, nil); err != nil {
				log.Printf("call: reap %s: %v", c.ID, err)
				continue
			}
			ended++
			continue
		}
		if _, rerr := h.rooms.ListParticipants(ctx, node, c.ID); errors.Is(rerr, out.ErrRoomNotFound) {
			if err := finishCall(ctx, h.writer, h.convs, h.events, c, nil); err != nil {
				log.Printf("call: reap missing room %s: %v", c.ID, err)
				continue
			}
			ended++
		}
	}
	return ended, nil
}
