package sfu

import (
	"errors"
	"io"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// rtpBufferSize is the read buffer for one forwarded RTP packet. 1500 covers
// an Ethernet MTU, which is the ceiling Pion's SRTP session enforces anyway.
const rtpBufferSize = 1500

// sdpTokenSafe strips anything outside what an SDP token (the grammar
// msid — RFC 8830 — and track/stream ids are built from) actually allows.
// sourceStreamID and sourceTrackID come straight from a publisher's own
// browser/OS (MediaStream.id / MediaStreamTrack.id) — usually a clean UUID,
// but not guaranteed: a Windows-sourced device id can carry brace-wrapped
// COM GUID formatting, for one. Forwarding that raw string embeds it in the
// msid line of every outgoing SDP the SFU builds for every OTHER
// participant, and nothing says every receiving browser's own SDP parser
// round-trips an unusual character identically — one quietly normalizing it
// differently than another desyncs event.streams[0].id (what a viewer
// receives) from the roster's stream_id (what the SFU actually sent),
// which is exactly the kind of mismatch that makes isScreenShare's lookup
// find nothing and fall back to "camera" with no way to tell why. Sanitizing
// before it ever reaches an outgoing SDP removes the ambiguity outright,
// regardless of which browser disagrees with which about the original.
func sdpTokenSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return -1
		}
	}, s)
}

// forwardedStreamID keeps the publisher identity on every forwarded stream.
// Browsers may assign a fresh remote track ID during renegotiation, so the
// track ID alone is not a stable UI ownership key. The stream ID survives the
// SFU forwarding path and lets clients put camera and screen share into the
// same participant section.
func forwardedStreamID(userID uuid.UUID, sourceStreamID string) string {
	return "pm-" + userID.String() + "-" + sdpTokenSafe(sourceStreamID)
}

// forwardedTrackID carries publisher and resolved media kind in the
// browser-visible track ID, as a fallback signal for clients: unlike a
// MediaStream ID, a track ID is not merged or replaced by browsers during
// renegotiation, and callers here already know the real kind (see
// resolveKind) rather than guessing it from a stream id.
func forwardedTrackID(userID uuid.UUID, kind, sourceTrackID string) string {
	return "pm-" + userID.String() + "-" + kind + "-" + sdpTokenSafe(sourceTrackID)
}

// resolveKind decides a newly published track's mic/camera/screen label. The
// publisher's own offer is authoritative when it declared one for this mid
// (see Frame.TrackKinds) — that is the only reliable signal for "screen",
// since a MediaStream's id cannot be renamed in a way that reaches the SDP.
// Absent a declaration (an older client, or this track's mid not resolved),
// trackLabel's stream-id guess is the fallback, same as before this fix.
func resolveKind(pub *participant, remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) string {
	mid := midForReceiver(pub.pc, receiver)
	if mid != "" {
		if kind, ok := pub.trackKinds[mid]; ok {
			return kind
		}
	}
	return trackLabel(remote.Kind(), remote.StreamID())
}

// midForReceiver finds the mid of the transceiver that owns receiver. OnTrack
// hands us the TrackRemote and RTPReceiver but not the mid directly.
func midForReceiver(pc *webrtc.PeerConnection, receiver *webrtc.RTPReceiver) string {
	for _, tr := range pc.GetTransceivers() {
		if tr.Receiver() == receiver {
			return tr.Mid()
		}
	}
	return ""
}

// midOnConnection is midForReceiver's counterpart for the sending side: the
// mid of pc's transceiver currently sending local, if any. Used to tell one
// specific viewer which of ITS OWN transceivers a forwarded track landed on
// — see participant.info.
func midOnConnection(pc *webrtc.PeerConnection, local *webrtc.TrackLocalStaticRTP) string {
	for _, tr := range pc.GetTransceivers() {
		if sender := tr.Sender(); sender != nil && sender.Track() == local {
			return tr.Mid()
		}
	}
	return ""
}

// onPublish turns a newly arrived remote track into exactly one local track
// and fans that local out to every other participant. A single copy loop feeds
// it, no matter how many subscribers there are — that is the whole point of an
// SFU. Runs on the actor.
func (r *room) onPublish(pub *participant, remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	if r.closed || pub.role != "publisher" {
		return
	}
	if cur, ok := r.peers[pub.userID]; !ok || cur != pub {
		return // the publisher left (or reconnected) before the track landed
	}

	kind := resolveKind(pub, remote, receiver)
	local, err := webrtc.NewTrackLocalStaticRTP(remote.Codec().RTPCodecCapability, forwardedTrackID(pub.userID, kind, remote.ID()), forwardedStreamID(pub.userID, remote.StreamID()))
	if err != nil {
		log.Printf("sfu: room %s: new local track for %s: %v", r.callID, pub.userID, err)
		return
	}
	pub.published[remote.ID()] = local
	pub.remoteSSRC[remote.ID()] = remote.SSRC()
	pub.publishedKind[remote.ID()] = kind
	if mid := midForReceiver(pub.pc, receiver); mid != "" {
		pub.midToSourceID[mid] = remote.ID()
	}

	for id, sub := range r.peers {
		if id != pub.userID {
			r.subscribe(sub, pub, local)
		}
	}

	go r.pump(remote, local)
	r.broadcastPeers()
	r.flushRenegotiation()
}

// subscribe attaches one publisher's local track to one subscriber's
// PeerConnection and marks the subscriber for renegotiation. Runs on the actor.
func (r *room) subscribe(sub, pub *participant, local *webrtc.TrackLocalStaticRTP) {
	if pub.role != "publisher" || (pub.muted && local.Kind() == webrtc.RTPCodecTypeAudio) {
		return
	}
	for _, sender := range sub.pc.GetSenders() {
		if sender.Track() == local {
			return
		}
	}
	sender, err := sub.pc.AddTrack(local)
	if err != nil {
		log.Printf("sfu: room %s: add track %s to %s: %v", r.callID, local.ID(), sub.userID, err)
		return
	}
	go drainRTCP(sender)
	sub.renegoNeeded = true

	// A new video subscriber joins mid-stream, so it needs a keyframe now
	// rather than at the publisher's next natural one.
	if local.Kind() == webrtc.RTPCodecTypeVideo {
		r.requestKeyframe(pub, local)
	}
}

// unsubscribeAll detaches a local track from every participant that has it and
// marks them dirty. Runs on the actor; the caller flushes renegotiation.
func (r *room) unsubscribeAll(local *webrtc.TrackLocalStaticRTP) {
	for _, p := range r.peers {
		for _, sender := range p.pc.GetSenders() {
			track, ok := sender.Track().(*webrtc.TrackLocalStaticRTP)
			if !ok || track != local {
				continue
			}
			if err := p.pc.RemoveTrack(sender); err != nil {
				log.Printf("sfu: room %s: remove track %s from %s: %v", r.callID, local.ID(), p.userID, err)
				continue
			}
			p.renegoNeeded = true
		}
	}
}

// dropLocal is the copy loop's epilogue: the publisher's track ended, so it
// comes off every subscriber and everyone renegotiates. Runs on the actor.
func (r *room) dropLocal(local *webrtc.TrackLocalStaticRTP) {
	if r.closed {
		return
	}
	for _, p := range r.peers {
		for sourceID, cur := range p.published {
			if cur != local {
				continue
			}
			delete(p.published, sourceID)
			delete(p.remoteSSRC, sourceID)
			delete(p.publishedKind, sourceID)
			for mid, sid := range p.midToSourceID {
				if sid == sourceID {
					delete(p.midToSourceID, mid)
				}
			}
		}
	}
	r.unsubscribeAll(local)
	r.broadcastPeers()
	r.flushRenegotiation()
}

// pump is the one copy loop per published track: read RTP off the publisher's
// remote track, write it to the local, which Pion fans out to every attached
// sender. Runs off the actor.
//
// Only a read failure ends the loop. TrackLocalStaticRTP.Write fans out to
// every binding and returns an aggregate error, so a single subscriber whose
// SRTP session died would otherwise take the publisher's track away from the
// whole room permanently — OnTrack never fires again for it. Write errors are
// logged once and the loop carries on; the dead subscriber is cleaned up by its
// own connection state change.
func (r *room) pump(remote *webrtc.TrackRemote, local *webrtc.TrackLocalStaticRTP) {
	buf := make([]byte, rtpBufferSize)
	warned := false
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			break
		}
		if _, err := local.Write(buf[:n]); err != nil && !errors.Is(err, io.ErrClosedPipe) && !warned {
			warned = true
			log.Printf("sfu: room %s: forwarding %s (continuing): %v", r.callID, local.ID(), err)
		}
	}
	r.do(func() { r.dropLocal(local) })
}

// requestKeyframe asks a publisher for an immediate keyframe. Runs on the
// actor; WriteRTCP does not block on the network.
func (r *room) requestKeyframe(pub *participant, local *webrtc.TrackLocalStaticRTP) {
	var ssrc webrtc.SSRC
	for sourceID, published := range pub.published {
		if published == local {
			ssrc = pub.remoteSSRC[sourceID]
			break
		}
	}
	if ssrc == 0 {
		return
	}
	pkt := &rtcp.PictureLossIndication{MediaSSRC: uint32(ssrc)}
	if err := pub.pc.WriteRTCP([]rtcp.Packet{pkt}); err != nil {
		log.Printf("sfu: room %s: pli to %s: %v", r.callID, pub.userID, err)
	}
}

// drainRTCP consumes the receiver reports and NACKs a sender gets back. Not
// reading them stalls Pion's interceptor chain for that sender.
func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, rtpBufferSize)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}
