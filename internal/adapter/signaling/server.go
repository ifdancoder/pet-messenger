// Package signaling serves the media-service's /rtc WebSocket: verify a
// join-ticket, hand the connection to the SFU room actor as a Signaller, and
// pump inbound frames into it until the socket closes.
package signaling

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"pet-messenger/internal/adapter/sfu"
	"pet-messenger/internal/config"
	infraauth "pet-messenger/internal/infrastructure/auth"
)

// writeTimeout bounds one frame write so a stalled client can't wedge the
// room actor, which calls Send synchronously via participant.send.
const writeTimeout = 5 * time.Second

type Server struct {
	mgr    sfu.Manager
	verify *infraauth.JoinTicketVerifier
	cfg    *config.Config
}

func NewServer(mgr sfu.Manager, verify *infraauth.JoinTicketVerifier, cfg *config.Config) *Server {
	return &Server{mgr: mgr, verify: verify, cfg: cfg}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rtc", s.handleRTC)
	return mux
}

func (s *Server) handleRTC(w http.ResponseWriter, r *http.Request) {
	claims, err := s.verify.Verify(r.URL.Query().Get("ticket"))
	if err != nil {
		http.Error(w, "bad ticket", http.StatusUnauthorized)
		return
	}
	if claims.NodeID != config.MediaNodeID(s.cfg) {
		http.Error(w, "wrong node", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.cfg.CORS.OriginHostPatterns(),
	})
	if err != nil {
		return
	}

	sig := newWSSignaller(conn)
	session, err := s.mgr.JoinSession(claims.CallID, claims.UserID, string(claims.Role), sig)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "join rejected")
		return
	}
	s.readLoop(r.Context(), conn, session)
}

// readLoop pumps raw client frames into the room actor until the socket
// errors or closes; either one means the participant left.
func (s *Server) readLoop(ctx context.Context, conn *websocket.Conn, session sfu.Session) {
	defer session.Close()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		session.ClientFrame(data)
	}
}

// wsSignaller adapts a *websocket.Conn to sfu.Signaller. Send is called from
// the room actor goroutine, never concurrently with itself, but a mutex keeps
// it safe if that ever changes and costs nothing on the hot path.
type wsSignaller struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func newWSSignaller(conn *websocket.Conn) *wsSignaller {
	return &wsSignaller{conn: conn}
}

func (s *wsSignaller) Send(frame []byte) error {
	if frame == nil {
		return nil // a marshal failure upstream already logged; nothing to send
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return s.conn.Write(ctx, websocket.MessageText, frame)
}

func (s *wsSignaller) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.Close(websocket.StatusNormalClosure, "bye")
}
