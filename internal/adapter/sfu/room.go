package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"

	"pet-messenger/internal/config"
)

const (
	// idleTimeout is how long a room with no participants lingers before it
	// closes itself and reports the call ended.
	idleTimeout = 30 * time.Second
	// cmdBuffer sizes the actor's command channel. Pion callbacks and WS reads
	// enqueue here, so it is generous enough that a burst of trickled
	// candidates never blocks a callback goroutine.
	cmdBuffer = 256
)

// room is an actor. Exactly one goroutine — loop — runs every command, so all
// peer and track state is single-threaded and needs no locks. Pion callbacks,
// the manager's control-plane calls and the WS read path all marshal work onto
// cmds.
type room struct {
	callID    uuid.UUID
	cfg       *config.Config
	api       *webrtc.API
	events    CallEventPublisher
	onDrained func(*room)

	cmds chan func()
	done chan struct{}

	// Below here: actor goroutine only.
	mutedUsers map[uuid.UUID]bool
	peers      map[uuid.UUID]*participant
	closed     bool
}

func newRoom(callID uuid.UUID, cfg *config.Config, api *webrtc.API, events CallEventPublisher, onDrained func(*room)) *room {
	r := &room{
		callID:     callID,
		cfg:        cfg,
		api:        api,
		events:     events,
		onDrained:  onDrained,
		cmds:       make(chan func(), cmdBuffer),
		done:       make(chan struct{}),
		peers:      map[uuid.UUID]*participant{},
		mutedUsers: map[uuid.UUID]bool{},
	}
	go r.loop()
	return r
}

func (r *room) loop() {
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-r.done:
			return
		case fn := <-r.cmds:
			fn()
			if r.closed {
				return
			}
			if len(r.peers) == 0 {
				idle.Reset(idleTimeout)
			} else {
				idle.Stop()
			}
		case <-idle.C:
			if len(r.peers) == 0 {
				r.drain()
				return
			}
		}
	}
}

// do enqueues fn on the actor. It is a no-op once the room has shut down.
func (r *room) do(fn func()) {
	select {
	case r.cmds <- fn:
	case <-r.done:
	}
}

// ask runs fn on the actor and waits for its reply. zero is returned when the
// room shut down before fn could run.
func ask[T any](r *room, fn func() T) T {
	reply := make(chan T, 1)
	r.do(func() { reply <- fn() })
	select {
	case v := <-reply:
		return v
	case <-r.done:
		select {
		case v := <-reply:
			return v
		default:
			var zero T
			return zero
		}
	}
}

// askError preserves a closed-room failure instead of returning a nil error
// when shutdown prevents the actor from executing the command.
func askError(r *room, fn func() error) error {
	result := ask(r, func() *error { err := fn(); return &err })
	if result == nil {
		return ErrRoomNotFound
	}
	return *result
}

type roomSession struct {
	room        *room
	participant *participant
}

func (s *roomSession) ClientFrame(frame []byte) {
	var f Frame
	if err := json.Unmarshal(frame, &f); err != nil {
		return
	}
	s.room.do(func() {
		if s.room.peers[s.participant.userID] != s.participant {
			return
		}
		s.room.dispatchClientFrame(s.participant.userID, f)
	})
}

func (s *roomSession) Close() {
	s.room.do(func() {
		if s.room.peers[s.participant.userID] == s.participant {
			s.room.removeParticipant(s.participant.userID)
		}
	})
}

// --- control-plane entry points (called from the manager, off the actor) ---

func (r *room) count() int {
	return ask(r, func() int { return len(r.peers) })
}

func (r *room) join(userID uuid.UUID, role string, sig Signaller) error {
	return askError(r, func() error { return r.addParticipant(userID, role, sig) })
}

func (r *room) mute(userID uuid.UUID) error {
	return askError(r, func() error { return r.muteParticipant(userID) })
}

func (r *room) remove(userID uuid.UUID) error {
	return askError(r, func() error { return r.kickParticipant(userID) })
}

func (r *room) close() {
	r.do(r.shutdown)
}

// handleClientFrame decodes one signalling frame from a client and dispatches
// it onto the actor. Task 12's WS read loop calls this for every message.
func (r *room) handleClientFrame(userID uuid.UUID, frame []byte) {
	var f Frame
	if err := json.Unmarshal(frame, &f); err != nil {
		log.Printf("sfu: room %s: bad frame from %s: %v", r.callID, userID, err)
		return
	}
	r.do(func() { r.dispatchClientFrame(userID, f) })
}

func (r *room) dispatchClientFrame(userID uuid.UUID, f Frame) {
	switch f.Type {
	case FrameOffer:
		r.handleOffer(userID, f.SDP, f.TrackKinds)
	case FrameAnswer:
		r.handleAnswer(userID, f.SDP)
	case FrameCandidate:
		r.handleCandidate(userID, f.Candidate)
	case FrameBye:
		r.removeParticipant(userID)
	default:
		log.Printf("sfu: room %s: unknown frame type %q from %s", r.callID, f.Type, userID)
	}
}

// --- actor-only ---

func (r *room) addParticipant(userID uuid.UUID, role string, sig Signaller) error {
	if r.closed {
		return ErrRoomNotFound
	}
	previous := r.peers[userID]
	if max := r.cfg.Media.MaxRoomPeers; previous == nil && max > 0 && len(r.peers) >= max {
		return ErrRoomFull
	}
	pc, err := r.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return fmt.Errorf("new peer connection: %w", err)
	}
	if previous != nil {
		r.detachParticipant(previous)
	}
	p := newParticipant(userID, role, sig, pc)
	p.muted = r.mutedUsers[userID]

	pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil {
			return
		}
		init := cand.ToJSON()
		r.do(func() { p.send(candidateFrame(init)) })
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed,
			webrtc.PeerConnectionStateDisconnected:
			r.do(func() {
				// Only the session that owns this pc leaves; a reconnect that
				// already replaced it must survive.
				if cur, ok := r.peers[userID]; ok && cur == p {
					r.removeParticipant(userID)
				}
			})
		default:
		}
	})
	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		r.do(func() { r.onPublish(p, remote, receiver) })
	})

	r.peers[userID] = p
	// Subscribe the newcomer to everything already being published.
	for id, other := range r.peers {
		if id == userID {
			continue
		}
		for _, local := range other.published {
			r.subscribe(p, other, local)
		}
	}

	if previous == nil {
		r.events.ParticipantJoined(r.callID, userID, role)
	}
	r.broadcastPeers()
	r.flushRenegotiation()
	return nil
}

// muteParticipant drops the participant's audio locals from every subscriber.
// Tracks stay registered so only an explicit moderator unmute restores them.
func (r *room) muteParticipant(userID uuid.UUID) error {
	p, ok := r.peers[userID]
	if !ok {
		return ErrParticipantNotInRoom
	}
	p.muted = true
	r.mutedUsers[userID] = true
	for _, local := range p.published {
		if local.Kind() != webrtc.RTPCodecTypeAudio {
			continue
		}
		r.unsubscribeAll(local)
	}
	p.send(mutedFrame(actorServer))
	r.broadcastPeers()
	r.flushRenegotiation()
	return nil
}

// unmuteParticipant restores audio forwarding after moderator approval.
func (r *room) unmuteParticipant(userID uuid.UUID) error {
	p, ok := r.peers[userID]
	if !ok {
		return ErrParticipantNotInRoom
	}
	if !p.muted {
		return nil
	}
	p.muted = false
	delete(r.mutedUsers, userID)
	for _, local := range p.published {
		if local.Kind() != webrtc.RTPCodecTypeAudio {
			continue
		}
		for id, sub := range r.peers {
			if id != userID {
				r.subscribe(sub, p, local)
			}
		}
	}
	p.send(marshalFrame(Frame{Type: FrameUnmuted, By: actorServer}))
	r.broadcastPeers()
	r.flushRenegotiation()
	return nil
}

// kickParticipant tells the participant it was removed, then tears it down.
func (r *room) kickParticipant(userID uuid.UUID) error {
	p, ok := r.peers[userID]
	if !ok {
		return ErrParticipantNotInRoom
	}
	p.send(removedFrame(actorServer))
	r.removeParticipant(userID)
	return nil
}

// removeParticipant drops a participant and everything it published, then
// renegotiates whoever was subscribed to it. The last one out drains the room.
func (r *room) removeParticipant(userID uuid.UUID) {
	p, ok := r.peers[userID]
	if !ok {
		return
	}
	r.detachParticipant(p)

	r.events.ParticipantLeft(r.callID, userID)
	if len(r.peers) == 0 {
		r.drain()
		return
	}
	r.broadcastPeers()
	r.flushRenegotiation()
}

// detachParticipant releases a session without treating replacement as a leave.
func (r *room) detachParticipant(p *participant) {
	delete(r.peers, p.userID)
	for id, local := range p.published {
		delete(p.published, id)
		delete(p.remoteSSRC, id)
		r.unsubscribeAll(local)
	}
	go p.teardown()
}

// flushRenegotiation sends one server offer to every participant whose set of
// forwarded tracks changed, but only where that is glare-safe: no outstanding
// server offer and a stable signalling state. Anything skipped keeps its dirty
// flag and is picked up by the next flush.
func (r *room) flushRenegotiation() {
	for _, p := range r.peers {
		if !p.negotiated || !p.renegoNeeded || p.signalingBusy {
			continue
		}
		if p.pc.SignalingState() != webrtc.SignalingStateStable {
			continue
		}
		offer, err := p.pc.CreateOffer(nil)
		if err != nil {
			log.Printf("sfu: room %s: create offer for %s: %v", r.callID, p.userID, err)
			continue
		}
		if err := p.pc.SetLocalDescription(offer); err != nil {
			log.Printf("sfu: room %s: set local offer for %s: %v", r.callID, p.userID, err)
			continue
		}
		p.send(offerFrame(offer))
		p.signalingBusy = true
		p.renegoNeeded = false
	}
}

func (r *room) handleOffer(userID uuid.UUID, sdp string, trackKinds map[string]string) {
	p, ok := r.peers[userID]
	if !ok {
		return
	}
	// Recorded before the glare check and before SetRemoteDescription: a
	// parked offer is replayed later through this same function, and OnTrack
	// can fire as soon as the remote description is set, so the declared kind
	// must already be in place by then. It can also retroactively fix a label
	// that was already published under resolveKind's guess — broadcast that
	// correction immediately even if this offer itself ends up parked below.
	if p.declareTrackKinds(trackKinds) {
		r.broadcastPeers()
	}
	if p.signalingBusy {
		// Glare: our own offer is still outstanding. Park the client's offer
		// and replay it once the answer lands.
		p.pendingOffer = sdp
		return
	}
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
		log.Printf("sfu: room %s: set remote offer from %s: %v", r.callID, userID, err)
		p.send(errorFrame("invalid offer"))
		return
	}
	p.flushCandidates()

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("sfu: room %s: create answer for %s: %v", r.callID, userID, err)
		return
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		log.Printf("sfu: room %s: set local answer for %s: %v", r.callID, userID, err)
		return
	}
	p.send(answerFrame(answer))
	p.negotiated = true
	// renegoNeeded is deliberately not cleared here: the client built this
	// offer from its own view, which may predate tracks the room added since,
	// so the server still owes it an offer of its own.
	r.flushRenegotiation()
}

func (r *room) handleAnswer(userID uuid.UUID, sdp string) {
	p, ok := r.peers[userID]
	if !ok {
		return
	}
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}); err != nil {
		log.Printf("sfu: room %s: set remote answer from %s: %v", r.callID, userID, err)
		return
	}
	p.signalingBusy = false
	p.flushCandidates()

	if parked := p.pendingOffer; parked != "" {
		p.pendingOffer = ""
		// Its track_kinds were already merged in when the offer first arrived
		// and got parked (see handleOffer) — nothing new to declare here.
		r.handleOffer(userID, parked, nil)
		return
	}
	r.flushRenegotiation()
}

func (r *room) handleCandidate(userID uuid.UUID, raw json.RawMessage) {
	p, ok := r.peers[userID]
	if !ok {
		return
	}
	var cand webrtc.ICECandidateInit
	if err := json.Unmarshal(raw, &cand); err != nil {
		log.Printf("sfu: room %s: bad candidate from %s: %v", r.callID, userID, err)
		return
	}
	p.addCandidate(cand)
}

// broadcastPeers pushes the full roster at everyone. Cheap enough at call
// sizes, and it keeps clients from having to reconcile deltas.
func (r *room) broadcastPeers() {
	roster := make([]PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		roster = append(roster, p.info())
	}
	sort.Slice(roster, func(i, j int) bool { return roster[i].UserID < roster[j].UserID })
	frame := peersFrame(roster)
	for _, p := range r.peers {
		p.send(frame)
	}
}

// drain closes an emptied room and reports the call ended.
func (r *room) drain() {
	r.shutdown()
	if r.onDrained != nil {
		r.onDrained(r)
	}
}

// shutdown tears every participant down and stops the actor. It fires no
// events — the caller decides whether this was a drain or a control-plane
// CloseRoom.
func (r *room) shutdown() {
	if r.closed {
		return
	}
	r.closed = true
	for id, p := range r.peers {
		delete(r.peers, id)
		go p.teardown()
	}
	close(r.done)
}
