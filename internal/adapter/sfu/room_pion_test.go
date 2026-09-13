//go:build !short

package sfu

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"pet-messenger/internal/config"
)

// This file drives the room with two real webrtc.PeerConnections — no
// WebSocket, no signalling server: fakeSig stands in for the transport and the
// test feeds frames back through room.handleClientFrame, exactly where Task
// 12's WS read loop will.

const (
	// iceTimeout is generous: host-candidate ICE on loopback is fast, but a
	// loaded CI box plus DTLS can still take a few seconds.
	iceTimeout   = 20 * time.Second
	mediaTimeout = 20 * time.Second
)

// --- test doubles -----------------------------------------------------------

// fakeSig is an in-memory Signaller: it decodes each frame and parks it on a
// channel the test's client loop drains.
type fakeSig struct {
	frames chan Frame
	mu     sync.Mutex
	closed bool
}

func newFakeSig() *fakeSig { return &fakeSig{frames: make(chan Frame, 128)} }

func (f *fakeSig) Send(frame []byte) error {
	var fr Frame
	if err := json.Unmarshal(frame, &fr); err != nil {
		return err
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return errors.New("fakeSig: closed")
	}
	select {
	case f.frames <- fr:
		return nil
	default:
		return errors.New("fakeSig: buffer full")
	}
}

func (f *fakeSig) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

// client is one browser-shaped peer: a real PeerConnection plus the loop that
// answers the server's frames.
type client struct {
	t      *testing.T
	pc     *webrtc.PeerConnection
	sig    *fakeSig
	mgr    Manager
	callID uuid.UUID
	userID uuid.UUID

	serverOffers chan struct{}
	muted        chan Frame
	removed      chan Frame
	peers        chan Frame
}

func newClient(t *testing.T, api *webrtc.API, mgr Manager, callID, userID uuid.UUID) *client {
	t.Helper()
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("client %s: NewPeerConnection: %v", userID, err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return &client{
		t: t, pc: pc, sig: newFakeSig(), mgr: mgr, callID: callID, userID: userID,
		serverOffers: make(chan struct{}, 8),
		muted:        make(chan Frame, 8),
		removed:      make(chan Frame, 8),
		peers:        make(chan Frame, 32),
	}
}

// run drains the signalling channel until ctx is done. Everything the server
// sends is applied to the real PeerConnection.
func (c *client) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case fr := <-c.sig.frames:
			c.handle(ctx, fr)
		}
	}
}

func (c *client) handle(ctx context.Context, fr Frame) {
	switch fr.Type {
	case FrameOffer:
		// A server renegotiation: the forwarded track set changed.
		if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: fr.SDP}); err != nil {
			c.t.Logf("client %s: set remote offer: %v", c.userID, err)
			return
		}
		answer, err := c.pc.CreateAnswer(nil)
		if err != nil {
			c.t.Logf("client %s: create answer: %v", c.userID, err)
			return
		}
		if err := c.pc.SetLocalDescription(answer); err != nil {
			c.t.Logf("client %s: set local answer: %v", c.userID, err)
			return
		}
		c.awaitGathering(ctx)
		c.mgr.ClientFrame(c.callID, c.userID, answerFrame(*c.pc.LocalDescription()))
		select {
		case c.serverOffers <- struct{}{}:
		default:
		}
	case FrameAnswer:
		if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: fr.SDP}); err != nil {
			c.t.Logf("client %s: set remote answer: %v", c.userID, err)
		}
	case FrameCandidate:
		var cand webrtc.ICECandidateInit
		if err := json.Unmarshal(fr.Candidate, &cand); err != nil {
			c.t.Logf("client %s: bad candidate: %v", c.userID, err)
			return
		}
		if err := c.pc.AddICECandidate(cand); err != nil {
			c.t.Logf("client %s: add candidate: %v", c.userID, err)
		}
	case FramePeers:
		select {
		case c.peers <- fr:
		default:
		}
	case FrameMuted:
		select {
		case c.muted <- fr:
		default:
		}
	case FrameRemoved:
		select {
		case c.removed <- fr:
		default:
		}
		// A kicked client hangs its own PeerConnection up; the server has
		// already closed its side.
		_ = c.pc.Close()
	default:
		c.t.Logf("client %s: unexpected frame %q (%s)", c.userID, fr.Type, fr.Message)
	}
}

// offer performs the initial client-is-the-offerer exchange. Gathering is run
// to completion so the offer carries its own candidates and the test does not
// have to trickle in both directions.
func (c *client) offer(ctx context.Context) {
	c.offerWithKinds(ctx, nil)
}

// offerWithKinds is offer plus a client_kinds declaration, exactly as the
// real browser client will send one alongside a screen-share offer: kinds
// maps a local track to what it is, and the mid each track landed on is
// resolved here (mid is unknown until the offer is created), the same way a
// real client resolves it from RTCRtpTransceiver.mid after createOffer.
func (c *client) offerWithKinds(ctx context.Context, kinds map[webrtc.TrackLocal]string) {
	c.t.Helper()
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		c.t.Fatalf("client %s: CreateOffer: %v", c.userID, err)
	}
	if err := c.pc.SetLocalDescription(offer); err != nil {
		c.t.Fatalf("client %s: SetLocalDescription: %v", c.userID, err)
	}
	c.awaitGathering(ctx)
	var trackKinds map[string]string
	if len(kinds) > 0 {
		trackKinds = map[string]string{}
		for _, tr := range c.pc.GetTransceivers() {
			sender := tr.Sender()
			if sender == nil || tr.Mid() == "" {
				continue
			}
			if kind, ok := kinds[sender.Track()]; ok {
				trackKinds[tr.Mid()] = kind
			}
		}
	}
	frame, err := json.Marshal(Frame{Type: FrameOffer, SDP: c.pc.LocalDescription().SDP, TrackKinds: trackKinds})
	if err != nil {
		c.t.Fatalf("client %s: marshal offer: %v", c.userID, err)
	}
	c.mgr.ClientFrame(c.callID, c.userID, frame)
}

func (c *client) awaitGathering(ctx context.Context) {
	select {
	case <-webrtc.GatheringCompletePromise(c.pc):
	case <-ctx.Done():
	case <-time.After(iceTimeout):
	}
}

func (c *client) waitConnected() {
	c.t.Helper()
	waitFor(c.t, iceTimeout, "client "+c.userID.String()+" connected", func() bool {
		return c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected
	})
}

// --- helpers ----------------------------------------------------------------

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

func pionTestConfig() *config.Config {
	return &config.Config{Media: config.MediaConfig{}}
}

// writeSilence keeps a synthetic Opus track flowing. The SFU never decodes, so
// the payload only has to be packetizable.
func writeSilence(ctx context.Context, track *webrtc.TrackLocalStaticSample) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	payload := make([]byte, 60)
	for i := range payload {
		payload[i] = byte(i)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := track.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond}); err != nil {
				return
			}
		}
	}
}

// --- the test ---------------------------------------------------------------

func TestRoomForwardsRTPBetweenPionPeers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Pion forwarding test in short mode (needs ICE + DTLS)")
	}

	api, err := newWebRTCAPI()
	if err != nil {
		t.Fatalf("newWebRTCAPI: %v", err)
	}

	events := &fakeEvents{}
	mgr, err := NewManager(pionTestConfig(), events)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	callID := uuid.Must(uuid.NewV7())
	if err := mgr.CreateRoom(callID); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- A: the publisher -----------------------------------------------
	userA := uuid.Must(uuid.NewV7())
	clientA := newClient(t, api, mgr, callID, userA)
	audio, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio-a", "mic-a",
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample: %v", err)
	}
	senderA, err := clientA.pc.AddTrack(audio)
	if err != nil {
		t.Fatalf("A AddTrack: %v", err)
	}
	go drainRTCP(senderA)
	go clientA.run(ctx)

	if err := mgr.Join(callID, userA, "publisher", clientA.sig); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	clientA.offer(ctx)
	clientA.waitConnected()
	go writeSilence(ctx, audio)

	// --- B: the viewer --------------------------------------------------
	userB := uuid.Must(uuid.NewV7())
	clientB := newClient(t, api, mgr, callID, userB)
	if _, err := clientB.pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		t.Fatalf("B AddTransceiverFromKind: %v", err)
	}
	gotTrack := make(chan *webrtc.TrackRemote, 1)
	clientB.pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		select {
		case gotTrack <- remote:
		default:
		}
	})
	go clientB.run(ctx)

	if err := mgr.Join(callID, userB, "viewer", clientB.sig); err != nil {
		t.Fatalf("Join B: %v", err)
	}
	clientB.offer(ctx)
	clientB.waitConnected()

	// The server must renegotiate with B to hand it A's track.
	select {
	case <-clientB.serverOffers:
	case <-time.After(mediaTimeout):
		t.Fatalf("no server renegotiation offer reached B within %s", mediaTimeout)
	}

	var remote *webrtc.TrackRemote
	select {
	case remote = <-gotTrack:
	case <-time.After(mediaTimeout):
		t.Fatalf("B's OnTrack never fired within %s", mediaTimeout)
	}
	if got := remote.Codec().MimeType; got != webrtc.MimeTypeOpus {
		t.Fatalf("forwarded track mime = %q, want %q", got, webrtc.MimeTypeOpus)
	}

	// The actual forwarding assertion: RTP that A wrote comes out of B.
	if err := remote.SetReadDeadline(time.Now().Add(mediaTimeout)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, rtpBufferSize)
	for i := 0; i < 5; i++ {
		n, _, err := remote.Read(buf)
		if err != nil {
			t.Fatalf("B read forwarded RTP packet %d: %v", i, err)
		}
		if n == 0 {
			t.Fatalf("B read an empty RTP packet at %d", i)
		}
	}

	if st := mgr.Stats(callID); !st.Exists || st.Participants != 2 {
		t.Fatalf("Stats = %+v, want {Participants:2 Exists:true}", st)
	}

	// --- moderation: kick B ---------------------------------------------
	if err := mgr.Remove(callID, userB); err != nil {
		t.Fatalf("Remove B: %v", err)
	}
	select {
	case fr := <-clientB.removed:
		if fr.By != actorServer {
			t.Fatalf("removed frame By = %q, want %q", fr.By, actorServer)
		}
	case <-time.After(mediaTimeout):
		t.Fatalf("B never got a removed frame")
	}
	waitFor(t, mediaTimeout, "B's PeerConnection closed", func() bool {
		return clientB.pc.ConnectionState() == webrtc.PeerConnectionStateClosed
	})
	waitFor(t, mediaTimeout, "the room down to one participant", func() bool {
		return mgr.Stats(callID).Participants == 1
	})

	// --- teardown --------------------------------------------------------
	if err := mgr.CloseRoom(callID); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}
	if !events.sawEnded(callID) {
		t.Fatalf("CloseRoom did not publish Ended(%s)", callID)
	}
	if st := mgr.Stats(callID); st.Exists {
		t.Fatalf("Stats after CloseRoom = %+v, want a zero value", st)
	}
}

// TestRoomLabelsTrackFromClientDeclaredKind is the regression test for a bug
// reported in production: every viewer saw a screen share captioned "Камера"
// (camera). Root cause — a client cannot rename a MediaStream's id in a way
// that reaches the SDP a real browser generates (Object.defineProperty only
// shadows the JS-visible property; the browser's own SDP generation reads its
// internal id, never that shadow), so trackLabel's "does the stream id say
// screen" heuristic can never actually see "screen" for a real screen share.
// This test's publisher uses a plain camera-shaped stream id — what a real
// browser always sends — and relies entirely on the offer's track_kinds
// declaration to end up labeled "screen" for every viewer.
func TestRoomLabelsTrackFromClientDeclaredKind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Pion forwarding test in short mode (needs ICE + DTLS)")
	}

	api, err := newWebRTCAPI()
	if err != nil {
		t.Fatalf("newWebRTCAPI: %v", err)
	}
	events := &fakeEvents{}
	mgr, err := NewManager(pionTestConfig(), events)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	callID := uuid.Must(uuid.NewV7())
	if err := mgr.CreateRoom(callID); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userA := uuid.Must(uuid.NewV7())
	clientA := newClient(t, api, mgr, callID, userA)
	video, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video-a", "camera-a", // deliberately camera-shaped: no "screen" anywhere
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample: %v", err)
	}
	senderA, err := clientA.pc.AddTrack(video)
	if err != nil {
		t.Fatalf("A AddTrack: %v", err)
	}
	go drainRTCP(senderA)
	go clientA.run(ctx)

	if err := mgr.Join(callID, userA, "publisher", clientA.sig); err != nil {
		t.Fatalf("Join A: %v", err)
	}
	clientA.offerWithKinds(ctx, map[webrtc.TrackLocal]string{video: LabelScreen})
	clientA.waitConnected()
	// The SFU's OnTrack fires on the first received RTP packet, not on
	// negotiation — without this, onPublish never runs and B never sees the
	// track at all, screen-labeled or not.
	go writeSilence(ctx, video)

	userB := uuid.Must(uuid.NewV7())
	clientB := newClient(t, api, mgr, callID, userB)
	if _, err := clientB.pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		t.Fatalf("B AddTransceiverFromKind: %v", err)
	}
	go clientB.run(ctx)
	if err := mgr.Join(callID, userB, "viewer", clientB.sig); err != nil {
		t.Fatalf("Join B: %v", err)
	}
	clientB.offer(ctx)
	clientB.waitConnected()

	deadline := time.After(mediaTimeout)
	for {
		select {
		case fr := <-clientB.peers:
			for _, peer := range fr.Peers {
				if peer.UserID != userA.String() {
					continue
				}
				for _, tr := range peer.Tracks {
					if tr.Label == LabelCamera {
						t.Fatalf("A's screen share was labeled %q for B — the bug this test guards against", tr.Label)
					}
					if tr.Label == LabelScreen {
						return
					}
				}
			}
		case <-deadline:
			t.Fatal("never saw A's track labeled \"screen\" in a peers frame reaching B")
		}
	}
}
