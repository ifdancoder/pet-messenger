package command

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
)

type removeParticipantHandler struct {
	calls out.CallLoader
	convs out.ConversationReader
	nodes out.MediaNodeRegistry
	rooms out.RoomControl
	parts out.ParticipantWriter
}

// NewRemoveParticipantHandler wires the RemoveParticipant use case.
func NewRemoveParticipantHandler(calls out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl, parts out.ParticipantWriter) in.RemoveParticipantUseCase {
	return &removeParticipantHandler{calls: calls, convs: convs, nodes: nodes, rooms: rooms, parts: parts}
}

// Handle removes cmd.TargetID from cmd.CallID, on cmd.Actor's authority,
// and marks the target's participant row left. See moderateParticipant for
// the shared preconditions.
func (h *removeParticipantHandler) Handle(ctx context.Context, cmd in.RemoveParticipantCommand) error {
	return moderateParticipant(ctx, h.calls, h.convs, h.nodes, cmd.Actor, cmd.CallID, cmd.TargetID, func(ctx context.Context, node out.Node, callID, targetID uuid.UUID) error {
		if err := h.rooms.RemoveParticipant(ctx, node, callID, targetID); err != nil {
			return err
		}
		if err := h.parts.MarkParticipantLeft(ctx, callID, targetID, time.Now().UTC()); err != nil {
			log.Printf("call: MarkParticipantLeft %s/%s: %v", callID, targetID, err)
		}
		return nil
	})
}
