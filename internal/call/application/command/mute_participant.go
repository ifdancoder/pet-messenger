package command

import (
	"context"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
)

type muteParticipantHandler struct {
	calls out.CallLoader
	convs out.ConversationReader
	nodes out.MediaNodeRegistry
	rooms out.RoomControl
}

// NewMuteParticipantHandler wires the MuteParticipant use case.
func NewMuteParticipantHandler(calls out.CallLoader, convs out.ConversationReader, nodes out.MediaNodeRegistry, rooms out.RoomControl) in.MuteParticipantUseCase {
	return &muteParticipantHandler{calls: calls, convs: convs, nodes: nodes, rooms: rooms}
}

// Handle mutes cmd.TargetID in cmd.CallID, on cmd.Actor's authority. See
// moderateParticipant for the shared preconditions.
func (h *muteParticipantHandler) Handle(ctx context.Context, cmd in.MuteParticipantCommand) error {
	return moderateParticipant(ctx, h.calls, h.convs, h.nodes, cmd.Actor, cmd.CallID, cmd.TargetID, h.rooms.MuteParticipant)
}
