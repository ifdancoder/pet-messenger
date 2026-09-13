package sfu

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/pion/webrtc/v4"
)

// Frame types carried over the /rtc WebSocket in both directions.
const (
	FrameOffer     = "offer"
	FrameAnswer    = "answer"
	FrameCandidate = "candidate"
	FramePeers     = "peers"
	FrameMuted     = "muted"
	FrameUnmuted   = "unmuted"
	FrameRemoved   = "removed"
	FrameError     = "error"
	FrameBye       = "bye"
)

// Track labels reported in a peers frame.
const (
	LabelMic    = "mic"
	LabelCamera = "camera"
	LabelScreen = "screen"
)

// actorServer is the value of a muted/removed frame's "by" field for actions
// driven by the api-service control plane rather than another participant.
const actorServer = "server"

// Frame is the signalling envelope. Only the fields a given type needs are
// populated; everything else is omitted.
type Frame struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Peers     []PeerInfo      `json:"peers,omitempty"`
	By        string          `json:"by,omitempty"`
	Message   string          `json:"message,omitempty"`
	// TrackKinds accompanies a client offer: mid -> one of LabelMic/
	// LabelCamera/LabelScreen, for every send transceiver the client knows the
	// kind of. The client is the only party that actually knows why a track
	// exists (getUserMedia vs. getDisplayMedia) — a MediaStream's id cannot be
	// renamed in a way that survives into the SDP the browser generates, so the
	// SFU cannot infer this from the stream id and must be told.
	TrackKinds map[string]string `json:"track_kinds,omitempty"`
}

// PeerInfo describes one participant in a peers frame.
type PeerInfo struct {
	UserID string      `json:"user_id"`
	Role   string      `json:"role"`
	Muted  bool        `json:"muted"`
	Tracks []TrackInfo `json:"tracks,omitempty"`
}

// TrackInfo describes one forwarded track of a participant.
type TrackInfo struct {
	Kind    string `json:"kind"`
	TrackID string `json:"track_id"`
	Label   string `json:"label"`
}

func offerFrame(desc webrtc.SessionDescription) []byte {
	return marshalFrame(Frame{Type: FrameOffer, SDP: desc.SDP})
}

func answerFrame(desc webrtc.SessionDescription) []byte {
	return marshalFrame(Frame{Type: FrameAnswer, SDP: desc.SDP})
}

func candidateFrame(cand webrtc.ICECandidateInit) []byte {
	raw, err := json.Marshal(cand)
	if err != nil {
		log.Printf("sfu: marshal ice candidate: %v", err)
		return nil
	}
	return marshalFrame(Frame{Type: FrameCandidate, Candidate: raw})
}

func peersFrame(peers []PeerInfo) []byte {
	return marshalFrame(Frame{Type: FramePeers, Peers: peers})
}

func mutedFrame(by string) []byte {
	return marshalFrame(Frame{Type: FrameMuted, By: by})
}

func removedFrame(by string) []byte {
	return marshalFrame(Frame{Type: FrameRemoved, By: by})
}

func errorFrame(message string) []byte {
	return marshalFrame(Frame{Type: FrameError, Message: message})
}

func marshalFrame(f Frame) []byte {
	b, err := json.Marshal(f)
	if err != nil {
		log.Printf("sfu: marshal %s frame: %v", f.Type, err)
		return nil
	}
	return b
}

// trackLabel is the fallback guess used only when the client did not declare
// a kind for this track's mid (an older client, or a race before its offer's
// track_kinds map was recorded). It cannot actually detect a screen share —
// see the TrackKinds doc comment — so it only ever resolves mic vs. camera
// correctly; a screen share falls through to "camera" here, same as before.
func trackLabel(kind webrtc.RTPCodecType, streamID string) string {
	if strings.Contains(strings.ToLower(streamID), LabelScreen) {
		return LabelScreen
	}
	if kind == webrtc.RTPCodecTypeAudio {
		return LabelMic
	}
	return LabelCamera
}

// isKnownLabel reports whether s is one of the three labels the client is
// allowed to declare in TrackKinds.
func isKnownLabel(s string) bool {
	return s == LabelMic || s == LabelCamera || s == LabelScreen
}
