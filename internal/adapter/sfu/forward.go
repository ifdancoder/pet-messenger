package sfu

import (
	"errors"
	"io"
	"log"

	"github.com/google/uuid"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// rtpBufferSize is the read buffer for one forwarded RTP packet. 1500 covers
// an Ethernet MTU, which is the ceiling Pion's SRTP session enforces anyway.
const rtpBufferSize = 1500

// forwardedStreamID keeps the publisher identity on every forwarded stream.
// Browsers may assign a fresh remote track ID during renegotiation, so the
// track ID alone is not a stable UI ownership key. The stream ID survives the
// SFU forwarding path and lets clients put camera and screen share into the
// same participant section.
func forwardedStreamID(userID uuid.UUID, sourceStreamID string) string {
	return "pm-" + userID.String() + "-" + sourceStreamID
}

// onPublish turns a newly arrived remote track into exactly one local track
// and fans that local out to every other participant. A single copy loop feeds
// it, no matter how many subscribers there are — that is the whole point of an
// SFU. Runs on the actor.
func (r *room) onPublish(pub *participant, remote *webrtc.TrackRemote) {
	if r.closed || pub.role != "publisher" {
		return
	}
	if cur, ok := r.peers[pub.userID]; !ok || cur != pub {
		return // the publisher left (or reconnected) before the track landed
	}

	local, err := webrtc.NewTrackLocalStaticRTP(remote.Codec().RTPCodecCapability, remote.ID(), forwardedStreamID(pub.userID, remote.StreamID()))
	if err != nil {
		log.Printf("sfu: room %s: new local track for %s: %v", r.callID, pub.userID, err)
		return
	}
	pub.published[remote.ID()] = local
	pub.remoteSSRC[remote.ID()] = remote.SSRC()

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
		r.requestKeyframe(pub, local.ID())
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
		if cur, ok := p.published[local.ID()]; ok && cur == local {
			delete(p.published, local.ID())
			delete(p.remoteSSRC, local.ID())
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
func (r *room) requestKeyframe(pub *participant, trackID string) {
	ssrc, ok := pub.remoteSSRC[trackID]
	if !ok {
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
