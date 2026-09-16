package sfu

import (
	"time"

	"github.com/pion/webrtc/v4"
)

// iceDisconnectedTimeout and iceFailedTimeout replace pion/ice's defaults
// (5s and 25s — a 30s initial-connection budget). A client with several
// network interfaces to enumerate (observed: a VPN can add a handful of
// virtual adapters) gathers and checks ICE candidates measurably slower
// than one with few — one real client's own relay candidate was still
// mid-gather well past the 30s mark. Pion's ICE agent enforces this
// entirely server-side and unilaterally: it starts counting down the
// instant it enters "checking", tears the connection down at the deadline
// regardless of what the remote side is still in the middle of doing, and
// never learns whether the remote would have succeeded a moment later.
// Widening the budget costs nothing for a client that finishes in under a
// second (the overwhelming common case observed) — it only changes how
// long a slow-gathering client is given before being cut off.
const (
	iceDisconnectedTimeout = 5 * time.Second
	iceFailedTimeout       = 115 * time.Second
)

// newWebRTCAPI builds the one API instance every room shares. Only Opus and
// VP8 are registered — the SFU forwards RTP as-is, so publisher and subscriber
// must agree on exactly these two.
func newWebRTCAPI() (*webrtc.API, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	se := webrtc.SettingEngine{}
	se.SetICETimeouts(iceDisconnectedTimeout, iceFailedTimeout, 2*time.Second)
	return webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se)), nil
}
