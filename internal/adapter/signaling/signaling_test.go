package signaling

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"pet-messenger/internal/adapter/sfu"
	"pet-messenger/internal/config"
	"pet-messenger/internal/domain/call"
	infraauth "pet-messenger/internal/infrastructure/auth"
)

// fakeManager records the calls a real sfu.Manager would receive, so the
// signalling server's plumbing is testable without a real WebRTC stack.
type fakeManager struct {
	mu         sync.Mutex
	joined     []uuid.UUID // userIDs
	joinErr    error
	frames     [][]byte
	removed    []uuid.UUID
	removedSig chan struct{}
}

func newFakeManager() *fakeManager { return &fakeManager{removedSig: make(chan struct{}, 8)} }

func (f *fakeManager) CreateRoom(uuid.UUID) error { return nil }
func (f *fakeManager) CloseRoom(uuid.UUID) error  { return nil }

func (f *fakeManager) Join(_, userID uuid.UUID, _ string, _ sfu.Signaller) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.joinErr != nil {
		return f.joinErr
	}
	f.joined = append(f.joined, userID)
	return nil
}

func (f *fakeManager) ClientFrame(_, _ uuid.UUID, frame []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, frame)
}

func (f *fakeManager) Mute(uuid.UUID, uuid.UUID) error { return nil }

func (f *fakeManager) Remove(_, userID uuid.UUID) error {
	f.mu.Lock()
	f.removed = append(f.removed, userID)
	f.mu.Unlock()
	f.removedSig <- struct{}{}
	return nil
}

func (f *fakeManager) Stats(uuid.UUID) sfu.RoomStats { return sfu.RoomStats{} }
func (f *fakeManager) Totals() sfu.Totals            { return sfu.Totals{} }

func (f *fakeManager) frameCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.frames)
}

func (f *fakeManager) joinCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.joined)
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return &config.Config{
		JWT:   config.JWTConfig{PrivateKey: priv, PublicKey: pub},
		Media: config.MediaConfig{NodeID: "node-under-test", JoinTicketTTL: time.Minute},
		CORS:  config.CORSConfig{AllowedOrigins: nil},
	}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func TestServer_JoinFrameRemove(t *testing.T) {
	cfg := testConfig(t)
	mgr := newFakeManager()
	verify := infraauth.NewJoinTicketVerifier(cfg)
	iss := infraauth.NewJoinTicketIssuer(cfg)

	srv := httptest.NewServer(NewServer(mgr, verify, cfg).Handler())
	defer srv.Close()

	callID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	tok, _, err := iss.Issue(callID, userID, call.RolePublisher, cfg.Media.NodeID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(srv.URL)+"/rtc?ticket="+tok, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"offer"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for mgr.frameCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if mgr.joinCount() != 1 {
		t.Fatalf("joinCount = %d, want 1", mgr.joinCount())
	}
	if mgr.frameCount() != 1 {
		t.Fatalf("frameCount = %d, want 1", mgr.frameCount())
	}

	_ = conn.Close(websocket.StatusNormalClosure, "done")
	select {
	case <-mgr.removedSig:
	case <-time.After(2 * time.Second):
		t.Fatal("Remove was never called after the socket closed")
	}
}

func TestServer_BadTicket_Unauthorized(t *testing.T) {
	cfg := testConfig(t)
	mgr := newFakeManager()
	srv := httptest.NewServer(NewServer(mgr, infraauth.NewJoinTicketVerifier(cfg), cfg).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/rtc?ticket=garbage")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestServer_WrongNode_Forbidden(t *testing.T) {
	cfg := testConfig(t)
	mgr := newFakeManager()
	iss := infraauth.NewJoinTicketIssuer(cfg)
	srv := httptest.NewServer(NewServer(mgr, infraauth.NewJoinTicketVerifier(cfg), cfg).Handler())
	defer srv.Close()

	tok, _, err := iss.Issue(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), call.RoleViewer, "some-other-node")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	resp, err := http.Get(srv.URL + "/rtc?ticket=" + tok)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if mgr.joinCount() != 0 {
		t.Fatalf("Join called for the wrong node: %d", mgr.joinCount())
	}
}
