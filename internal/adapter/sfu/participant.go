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
	// publishedKind remembers the resolved mic/camera/screen label for each
	// published track (same keys as published), so info() reports exactly what
	// onPublish decided instead of re-guessing from the stream id.
	publishedKind map[string]string

	// trackKinds is this participant's own claim, from its most recent offer,
	// of which mid carries which kind of media (see Frame.TrackKinds). Read by
	// onPublish when a track for that mid arrives.
	trackKinds map[string]string
	// midToSourceID remembers which already-published track (keyed the same
	// way as published/publishedKind) came in on which mid, so a track_kinds
	// declaration that arrives after onPublish already resolved a guess can
	// still go back and correct it — see declareTrackKinds.
	midToSourceID map[string]string

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
		userID:        userID,
		role:          role,
		sig:           sig,
		pc:            pc,
		published:     map[string]*webrtc.TrackLocalStaticRTP{},
		remoteSSRC:    map[string]webrtc.SSRC{},
		publishedKind: map[string]string{},
		trackKinds:    map[string]string{},
		midToSourceID: map[string]string{},
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

// declareTrackKinds records this participant's claim of what each of its
// mids carries, from the track_kinds field of its latest offer. Merged, never
// replaced: a mid's kind is stable for the life of its transceiver, and a
// later offer that omits a mid (the client only resends what changed) must
// not erase what an earlier one already declared.
//
// If a mid's track already published under a guessed label (resolveKind's
// fallback runs the moment OnTrack fires, whatever the reason its declaration
// hadn't arrived yet — client/server signalling ordering has no hard
// guarantee across every browser's WebRTC stack), a declaration that shows up
// afterward corrects the already-published label instead of leaving it wrong
// for the rest of that track's life. Reports whether it corrected anything,
// so the caller knows to broadcast the fix.
func (p *participant) declareTrackKinds(kinds map[string]string) bool {
	corrected := false
	for mid, kind := range kinds {
		if !isKnownLabel(kind) {
			continue
		}
		p.trackKinds[mid] = kind
		if sourceID, ok := p.midToSourceID[mid]; ok && p.publishedKind[sourceID] != kind {
			p.publishedKind[sourceID] = kind
			corrected = true
		}
	}
	return corrected
}

// info renders this participant for a peers frame, personalized for one
// specific viewer: each track's Mid is that viewer's own
// RTCRtpTransceiver.mid for it, found by searching the viewer's own
// PeerConnection — see TrackInfo for why this, rather than the shared
// StreamID, is the correlation key clients should actually rely on. Never
// meaningful for a participant's own entry in their own roster (nobody
// subscribes to their own tracks), so it's left empty there.
func (p *participant) info(viewer *participant) PeerInfo {
	out := PeerInfo{UserID: p.userID.String(), Role: p.role, Muted: p.muted}
	for id, local := range p.published {
		label := p.publishedKind[id]
		if label == "" {
			label = trackLabel(local.Kind(), local.StreamID())
		}
		track := TrackInfo{
			Kind:     local.Kind().String(),
			TrackID:  local.ID(),
			StreamID: local.StreamID(),
			Label:    label,
		}
		if viewer != p {
			track.Mid = midOnConnection(viewer.pc, local)
		}
		out.Tracks = append(out.Tracks, track)
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
