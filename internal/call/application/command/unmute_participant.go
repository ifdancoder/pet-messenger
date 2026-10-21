package command

import (
	"context"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
)

type unmuteParticipantHandler struct {
	calls out.CallLoader
	convs out.ConversationReader
	nodes out.MediaNodeRegistry
	rooms out.RoomControl
}

// NewUnmuteParticipantHandler wires the UnmuteParticipant use case.
func NewUnmuteParticipantHandler(calls out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl) in.UnmuteParticipantUseCase {
	return &unmuteParticipantHandler{calls: calls, convs: convs, nodes: nodes, rooms: rooms}
}

// Handle unmutes cmd.TargetID in cmd.CallID, on cmd.Actor's authority. See
// moderateParticipant for the shared preconditions.
func (h *unmuteParticipantHandler) Handle(ctx context.Context, cmd in.UnmuteParticipantCommand) error {
	return moderateParticipant(ctx, h.calls, h.convs, h.nodes, cmd.Actor, cmd.CallID, cmd.TargetID, h.rooms.UnmuteParticipant)
}
