//go:build !short

package sfu_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"pet-messenger/internal/platform/adapter/rpc"
	"pet-messenger/internal/platform/adapter/rpc/mediav1"
	"pet-messenger/internal/platform/adapter/sfu"
	"pet-messenger/internal/platform/adapter/signaling"
	call "pet-messenger/internal/call/domain"
	"pet-messenger/internal/platform/config"
	infraauth "pet-messenger/internal/platform/infrastructure/auth"
)

type callEvents struct{ ended chan uuid.UUID }

func (*callEvents) ParticipantJoined(uuid.UUID, uuid.UUID, string) {}
func (*callEvents) ParticipantLeft(uuid.UUID, uuid.UUID)           {}
func (e *callEvents) Ended(id uuid.UUID)                           { e.ended <- id }

func waitForClosed(t *testing.T, pc *webrtc.PeerConnection) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	for {
		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatal("removed peer connection did not close")
		}
	}
}

type rtcPeer struct {
	pc      *webrtc.PeerConnection
	ws      *websocket.Conn
	frames  chan sfu.Frame
	packets chan string
	errors  chan error
	answers chan struct{}
	mu      sync.Mutex
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

func newRTCPeer(t *testing.T, cfg *config.Config, url string, callID, userID uuid.UUID, role call.Role) *rtcPeer {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	p := &rtcPeer{pc: pc, frames: make(chan sfu.Frame, 128), packets: make(chan string, 1024), errors: make(chan error, 16), answers: make(chan struct{}, 8), cancel: cancel}
	t.Cleanup(func() {
		cancel()
		if p.ws != nil {
			_ = p.ws.CloseNow()
		}
		_ = pc.Close()
		p.wg.Wait()
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		p.wg.Go(func() {
			for {
				packet, _, err := track.ReadRTP()
				if err != nil {
					return
				}
				if len(packet.Payload) == 0 {
					p.fail(fmt.Errorf("empty RTP payload on %s", track.ID()))
					return
				}
				select {
				case p.packets <- track.ID():
				case <-ctx.Done():
					return
				}
			}
		})
	})
	ticket, _, err := infraauth.NewJoinTicketIssuer(cfg).Issue(callID, userID, role, config.MediaNodeID(cfg))
	if err != nil {
		t.Fatal(err)
	}
	p.ws, _, err = websocket.Dial(ctx, strings.Replace(url, "http", "ws", 1)+"/rtc?ticket="+ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.ws.SetReadLimit(1 << 20)
	p.wg.Go(func() {
		var pending []webrtc.ICECandidateInit
		for {
			_, data, err := p.ws.Read(ctx)
			if err != nil {
				return
			}
			var frame sfu.Frame
			if err := json.Unmarshal(data, &frame); err != nil {
				p.fail(err)
				return
			}
			p.mu.Lock()
			switch frame.Type {
			case sfu.FrameOffer, sfu.FrameAnswer:
				typ := webrtc.SDPTypeAnswer
				if frame.Type == sfu.FrameOffer {
					typ = webrtc.SDPTypeOffer
				}
				err = p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: typ, SDP: frame.SDP})
				if err == nil {
					for _, c := range pending {
						if e := pc.AddICECandidate(c); e != nil {
							err = e
							break
						}
					}
					pending = nil
				}
				if err == nil && typ == webrtc.SDPTypeOffer {
					err = p.describe(ctx, false)
				}
				if err == nil && typ == webrtc.SDPTypeAnswer {
					p.answers <- struct{}{}
				}
			case sfu.FrameCandidate:
				var c webrtc.ICECandidateInit
				err = json.Unmarshal(frame.Candidate, &c)
				if err == nil {
					if pc.RemoteDescription() == nil {
						pending = append(pending, c)
					} else {
						err = pc.AddICECandidate(c)
					}
				}
			case sfu.FrameError:
				err = fmt.Errorf("server signaling error: %s", frame.Message)
			case sfu.FrameRemoved:
				_ = pc.Close()
			}
			p.mu.Unlock()
			if err != nil {
				p.fail(fmt.Errorf("handle %s: %w", frame.Type, err))
				return
			}
			select {
			case p.frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	})
	return p
}
func (p *rtcPeer) fail(err error) {
	select {
	case p.errors <- err:
	default:
	}
}
func (p *rtcPeer) describe(ctx context.Context, offer bool) error {
	var desc webrtc.SessionDescription
	var err error
	if offer {
		desc, err = p.pc.CreateOffer(nil)
	} else {
		desc, err = p.pc.CreateAnswer(nil)
	}
	if err != nil {
		return err
	}
	gathered := webrtc.GatheringCompletePromise(p.pc)
	if err = p.pc.SetLocalDescription(desc); err != nil {
		return err
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return ctx.Err()
	}
	typ := sfu.FrameAnswer
	if offer {
		typ = sfu.FrameOffer
	}
	data, err := json.Marshal(sfu.Frame{Type: typ, SDP: p.pc.LocalDescription().SDP})
	if err != nil {
		return err
	}
	return p.ws.Write(ctx, websocket.MessageText, data)
}
func (p *rtcPeer) offer(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	p.mu.Lock()
	err := p.describe(ctx, true)
	p.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.answers:
	case err := <-p.errors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for answer")
	}
}
func (p *rtcPeer) addTrack(t *testing.T, id, stream, mime string) {
	t.Helper()
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: mime}, id, stream)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	sender, err := p.pc.AddTrack(track)
	p.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	p.wg.Go(func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	})
	p.wg.Go(func() {
		timer := time.NewTicker(20 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-t.Context().Done():
				return
			case <-timer.C:
				if p.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
					return
				}
				if err := track.WriteSample(media.Sample{Data: []byte{0x10, 0, 0, 1, 2, 3, 4, 5}, Duration: 20 * time.Millisecond}); err != nil {
					return
				}
			}
		}
	})
}
func (p *rtcPeer) frame(t *testing.T, predicate func(sfu.Frame) bool) sfu.Frame {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case f := <-p.frames:
			if predicate(f) {
				return f
			}
		case err := <-p.errors:
			t.Fatal(err)
		case <-timer.C:
			t.Fatal("timed out waiting for signaling frame")
		}
	}
}
// tracks waits for at least one RTP packet on each of ids. A forwarded
// track's real p.packets ID is never the bare id a caller publishes under
// (e.g. "audio-a"): the SFU renames it to forwardedTrackID's
// "pm-<publisher-id>-<kind>-<source-id>" scheme to carry publisher identity
// and resolved kind across the wire (see forward.go). The source id it was
// published under is always that scheme's suffix, so match on that instead
// of equality.
func (p *rtcPeer) tracks(t *testing.T, ids ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for len(want) > 0 {
		select {
		case id := <-p.packets:
			for w := range want {
				if strings.HasSuffix(id, w) {
					delete(want, w)
					break
				}
			}
		case err := <-p.errors:
			t.Fatal(err)
		case <-timer.C:
			t.Fatalf("missing RTP tracks: %v", want)
		}
	}
}

func setupRTC(t *testing.T) (*config.Config, string, uuid.UUID, sfu.Manager, mediav1.RoomControlClient, *callEvents) {
	t.Helper()
	if testing.Short() {
		t.Skip("requires real ICE, DTLS, WebSocket and gRPC")
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWT: config.JWTConfig{PrivateKey: priv, PublicKey: pub}, Media: config.MediaConfig{NodeID: "e2e", JoinTicketTTL: time.Minute, RPCToken: "test-secret"}}
	events := &callEvents{ended: make(chan uuid.UUID, 8)}
	mgr, err := sfu.NewManager(cfg, events)
	if err != nil {
		t.Fatal(err)
	}
	callID := uuid.New()
	t.Cleanup(func() { _ = mgr.CloseRoom(callID) })
	server := httptest.NewServer(signaling.NewServer(mgr, infraauth.NewJoinTicketVerifier(cfg), cfg).Handler())
	t.Cleanup(server.Close)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(rpc.UnaryServerAuthInterceptor(cfg.Media.RPCToken)))
	mediav1.RegisterRoomControlServer(grpcServer, rpc.NewRoomControlServer(mgr))
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(rpc.UnaryClientAuthInterceptor(cfg.Media.RPCToken)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	control := mediav1.NewRoomControlClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	if _, err := control.CreateRoom(ctx, &mediav1.CreateRoomRequest{CallId: callID.String()}); err != nil {
		t.Fatal(err)
	}
	return cfg, server.URL, callID, mgr, control, events
}

func TestSFUEndToEnd(t *testing.T) {
	cfg, url, callID, mgr, control, events := setupRTC(t)
	userA, userB := uuid.New(), uuid.New()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	a := newRTCPeer(t, cfg, url, callID, userA, call.RolePublisher)
	a.addTrack(t, "audio-a", "mic-a", webrtc.MimeTypeOpus)
	a.addTrack(t, "camera-a", "camera-a", webrtc.MimeTypeVP8)
	a.offer(t)
	a.frame(t, func(f sfu.Frame) bool {
		return f.Type == sfu.FramePeers && len(f.Peers) == 1 && len(f.Peers[0].Tracks) == 2
	})
	b := newRTCPeer(t, cfg, url, callID, userB, call.RolePublisher)
	b.addTrack(t, "audio-b", "mic-b", webrtc.MimeTypeOpus)
	b.offer(t)
	b.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameOffer })
	b.tracks(t, "audio-a", "camera-a")
	a.tracks(t, "audio-b")
	a.addTrack(t, "screen-a", "screen-a", webrtc.MimeTypeVP8)
	a.offer(t)
	b.frame(t, func(f sfu.Frame) bool {
		if f.Type != sfu.FramePeers {
			return false
		}
		for _, p := range f.Peers {
			if p.UserID != userA.String() {
				continue
			}
			labels := map[string]bool{}
			for _, tr := range p.Tracks {
				labels[tr.Label] = true
			}
			return labels["camera"] && labels["screen"]
		}
		return false
	})
	b.tracks(t, "screen-a")
	if _, err := control.MuteParticipant(ctx, &mediav1.MuteParticipantRequest{CallId: callID.String(), UserId: userA.String()}); err != nil {
		t.Fatal(err)
	}
	if f := a.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameMuted }); f.By != "server" {
		t.Fatalf("muted by %q", f.By)
	}
	// Wait for removal renegotiation, then drain packets already queued in transit.
	b.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameOffer })
	settle := time.NewTimer(300 * time.Millisecond)
settling:
	for {
		select {
		case <-b.packets:
		case <-settle.C:
			break settling
		}
	}
	silence := time.NewTimer(400 * time.Millisecond)
checking:
	for {
		select {
		case id := <-b.packets:
			if strings.HasSuffix(id, "audio-a") {
				t.Fatal("muted audio still forwarded")
			}
		case err := <-b.errors:
			t.Fatal(err)
		case <-silence.C:
			break checking
		}
	}
	b.tracks(t, "camera-a", "screen-a")
	if _, err := control.RemoveParticipant(ctx, &mediav1.RemoveParticipantRequest{CallId: callID.String(), UserId: userB.String()}); err != nil {
		t.Fatal(err)
	}
	if f := b.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameRemoved }); f.By != "server" {
		t.Fatalf("removed by %q", f.By)
	}
	waitForClosed(t, b.pc)
	st, err := control.RoomStats(ctx, &mediav1.RoomStatsRequest{CallId: callID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Exists || st.Participants != 1 {
		t.Fatalf("room stats: %v", st)
	}
	_ = a.ws.Close(websocket.StatusNormalClosure, "bye")
	select {
	case id := <-events.ended:
		if id != callID {
			t.Fatalf("ended call %s", id)
		}
	case <-ctx.Done():
		t.Fatal("room did not drain")
	}
	if mgr.Stats(callID).Exists {
		t.Fatal("drained room still exists")
	}
}

// A viewer must remain receive-only even when its client sends publishing SDP.
func TestSFUViewerCannotPublish(t *testing.T) {
	cfg, url, callID, _, _, _ := setupRTC(t)
	a := newRTCPeer(t, cfg, url, callID, uuid.New(), call.RolePublisher)
	a.addTrack(t, "audio-a", "mic-a", webrtc.MimeTypeOpus)
	a.offer(t)
	a.frame(t, func(f sfu.Frame) bool {
		return f.Type == sfu.FramePeers && len(f.Peers) == 1 && len(f.Peers[0].Tracks) == 1
	})
	viewer := newRTCPeer(t, cfg, url, callID, uuid.New(), call.RoleViewer)
	viewer.addTrack(t, "forbidden-audio", "mic-viewer", webrtc.MimeTypeOpus)
	viewer.offer(t)
	viewer.tracks(t, "audio-a")
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case id := <-a.packets:
			t.Fatalf("viewer published track %q", id)
		case err := <-a.errors:
			t.Fatal(err)
		case f := <-a.frames:
			for _, peer := range f.Peers {
				if peer.Role == "viewer" && len(peer.Tracks) > 0 {
					t.Fatal("viewer tracks advertised in roster")
				}
			}
		case <-timer.C:
			return
		}
	}
}

func (p *rtcPeer) noTrack(t *testing.T, forbidden string) {
	t.Helper()
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case id := <-p.packets:
			if strings.HasSuffix(id, forbidden) {
				t.Fatalf("forbidden RTP track %q", id)
			}
		case err := <-p.errors:
			t.Fatal(err)
		case <-timer.C:
			return
		}
	}
}

func TestSFUMuteRequiresModerator(t *testing.T) {
	cfg, url, callID, _, control, _ := setupRTC(t)
	userA := uuid.New()
	a := newRTCPeer(t, cfg, url, callID, userA, call.RolePublisher)
	a.addTrack(t, "original-audio", "mic-a", webrtc.MimeTypeOpus)
	a.offer(t)
	a.frame(t, func(f sfu.Frame) bool {
		return f.Type == sfu.FramePeers && len(f.Peers) == 1 && len(f.Peers[0].Tracks) == 1
	})
	b := newRTCPeer(t, cfg, url, callID, uuid.New(), call.RolePublisher)
	b.addTrack(t, "audio-b", "mic-b", webrtc.MimeTypeOpus)
	b.offer(t)
	b.tracks(t, "original-audio")
	a.tracks(t, "audio-b")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, err := control.MuteParticipant(ctx, &mediav1.MuteParticipantRequest{CallId: callID.String(), UserId: userA.String()}); err != nil {
		t.Fatal(err)
	}
	a.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameMuted })
	// A replacement connection and fresh track must inherit the moderator's mute.
	replacement := newRTCPeer(t, cfg, url, callID, userA, call.RolePublisher)
	replacement.addTrack(t, "replacement-audio", "mic-a", webrtc.MimeTypeOpus)
	replacement.addTrack(t, "replacement-camera", "camera-a", webrtc.MimeTypeVP8)
	replacement.offer(t)
	b.tracks(t, "replacement-camera")
	b.noTrack(t, "replacement-audio")
	// A subsequent client offer cannot clear mute either.
	replacement.offer(t)
	b.noTrack(t, "replacement-audio")
	// New subscribers must also respect the mute decision.
	c := newRTCPeer(t, cfg, url, callID, uuid.New(), call.RolePublisher)
	c.addTrack(t, "audio-c", "mic-c", webrtc.MimeTypeOpus)
	c.offer(t)
	c.tracks(t, "replacement-camera", "audio-b")
	c.noTrack(t, "replacement-audio")
	if _, err := control.UnmuteParticipant(ctx, &mediav1.UnmuteParticipantRequest{CallId: callID.String(), UserId: userA.String()}); err != nil {
		t.Fatal(err)
	}
	replacement.frame(t, func(f sfu.Frame) bool { return f.Type == sfu.FrameUnmuted && f.By == "server" })
	b.tracks(t, "replacement-audio")
	c.tracks(t, "replacement-audio")
}
