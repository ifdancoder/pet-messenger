package out

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrRoomNotFound means the media node is reachable but has no state for
// the given call. It is distinct from a transport failure: the call can
// safely be finished rather than merely retried later. Declared here,
// alongside RoomControl, because it is part of ListParticipants' own
// contract -- both the command package (ReapStaleCalls) and the query
// package (GetLiveForConversation) need to recognize it, and a port-level
// sentinel avoids the cross-sibling-package duplication that would
// otherwise be needed (contrast with finishCall's own duplication, which
// has no port to live on).
var ErrRoomNotFound = errors.New("call: media room not found")

// RoomControl is the gRPC client surface onto a media node's SFU room.
type RoomControl interface {
	CreateRoom(ctx context.Context, node Node, callID uuid.UUID) error
	CloseRoom(ctx context.Context, node Node, callID uuid.UUID) error
	MuteParticipant(ctx context.Context, node Node, callID, userID uuid.UUID) error
	UnmuteParticipant(ctx context.Context, node Node, callID, userID uuid.UUID) error
	RemoveParticipant(ctx context.Context, node Node, callID, userID uuid.UUID) error
	// ListParticipants returns ErrRoomNotFound when the node has no state
	// for callID.
	ListParticipants(ctx context.Context, node Node, callID uuid.UUID) ([]ParticipantInfo, error)
}

// ParticipantInfo is one entry in a room's live roster, as reported by the
// media node over gRPC (see RoomControl.ListParticipants).
type ParticipantInfo struct {
	UserID     string   `json:"user_id"`
	Role       string   `json:"role"`
	Muted      bool     `json:"muted"`
	SelfMuted  bool     `json:"self_muted"`
	TrackKinds []string `json:"track_kinds"` // "mic", "camera", "screen" -- deduplicated
	Connected  bool     `json:"connected"`
	Accepted   bool     `json:"accepted"`
}
