package sfu

import (
	"errors"
	"log"
	"sort"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

var (
	ErrParticipantNotInRoom = errors.New("sfu: participant not in room")
	ErrRoomFull             = errors.New("sfu: room is full")
)

// participant is one client's server-side state: its PeerConnection, the
// tracks it publishes (already converted to forwardable locals), its
// signalling channel and the renegotiation bookkeeping.
//
// Every field is owned by the room's actor goroutine. Nothing here may be
// touched from a Pion callback or a WS read loop directly — those marshal onto
// the room's command channel first.
type participant struct {
	userID uuid.UUID
	role   string
	muted  bool

	pc  *webrtc.PeerConnection
	sig Signaller

	// published maps a remote track id to the single local track the room
	// fans out to every other participant.
	published map[string]*webrtc.TrackLocalStaticRTP
	// remoteSSRC remembers each published track's inbound SSRC so the room can
	// address a PLI at it.
	remoteSSRC map[string]webrtc.SSRC

	// negotiated turns true once the client's initial offer has been answered.
	// The client is the initial offerer, so the server must not jump in with a
	// renegotiation offer before that first exchange finishes.
	negotiated bool
	// signalingBusy is true while a server-initiated offer is outstanding.
	signalingBusy bool
	// renegoNeeded is the dirty flag: the set of forwarded tracks changed and
	// this participant still has to be told about it.
	renegoNeeded bool
	// pendingOffer holds a client offer that arrived during glare (while a
	// server offer was outstanding); it is replayed once the answer lands.
	pendingOffer string
	// pendingCands buffers trickled candidates that arrived before the remote
	// description was set.
	pendingCands []webrtc.ICECandidateInit
}

func newParticipant(userID uuid.UUID, role string, sig Signaller, pc *webrtc.PeerConnection) *participant {
	return &participant{
		userID:     userID,
		role:       role,
		sig:        sig,
		pc:         pc,
		published:  map[string]*webrtc.TrackLocalStaticRTP{},
		remoteSSRC: map[string]webrtc.SSRC{},
	}
}

// send pushes one frame at the client. A signalling failure is logged and
// otherwise ignored — the connection state change will clean the peer up.
func (p *participant) send(frame []byte) {
	if p.sig == nil || len(frame) == 0 {
		return
	}
	if err := p.sig.Send(frame); err != nil {
		log.Printf("sfu: send %s: %v", p.userID, err)
	}
}

// addCandidate applies a trickled candidate, buffering it while the remote
// description is still missing.
func (p *participant) addCandidate(cand webrtc.ICECandidateInit) {
	if p.pc.RemoteDescription() == nil {
		p.pendingCands = append(p.pendingCands, cand)
		return
	}
	if err := p.pc.AddICECandidate(cand); err != nil {
		log.Printf("sfu: add ice candidate for %s: %v", p.userID, err)
	}
}

// flushCandidates applies everything buffered by addCandidate. Called right
// after a remote description lands.
func (p *participant) flushCandidates() {
	pending := p.pendingCands
	p.pendingCands = nil
	for _, cand := range pending {
		if err := p.pc.AddICECandidate(cand); err != nil {
			log.Printf("sfu: add buffered ice candidate for %s: %v", p.userID, err)
		}
	}
}

// info renders this participant for a peers frame.
func (p *participant) info() PeerInfo {
	out := PeerInfo{UserID: p.userID.String(), Role: p.role, Muted: p.muted}
	for id, local := range p.published {
		out.Tracks = append(out.Tracks, TrackInfo{
			Kind:    local.Kind().String(),
			TrackID: id,
			Label:   trackLabel(local.Kind(), local.StreamID()),
		})
	}
	sort.Slice(out.Tracks, func(i, j int) bool { return out.Tracks[i].TrackID < out.Tracks[j].TrackID })
	return out
}

// teardown closes the PeerConnection and the signalling channel. It is run off
// the actor goroutine so a slow Close never stalls the room.
func (p *participant) teardown() {
	if p.pc != nil {
		if err := p.pc.Close(); err != nil {
			log.Printf("sfu: close peer connection %s: %v", p.userID, err)
		}
	}
	if p.sig != nil {
		p.sig.Close()
	}
}
