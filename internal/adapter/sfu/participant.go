package sfu

import (
	"errors"

	"github.com/google/uuid"
)

var ErrParticipantNotInRoom = errors.New("sfu: participant not in room")

type participant struct {
	userID uuid.UUID
	role   string
	muted  bool
	// Task 11 adds: pc *webrtc.PeerConnection, ws, published tracks, etc.
}

func (p *participant) teardown() {
	// Task 11: close the PeerConnection and the WS.
}
