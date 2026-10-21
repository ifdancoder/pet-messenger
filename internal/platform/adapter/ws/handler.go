package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	infraauth "pet-messenger/internal/platform/infrastructure/auth"

	"pet-messenger/internal/conversation/domain"
)

const defaultConnBuffer = 64 // outbound frames buffered before a slow client is dropped

// MemberChecker is the slice of the conversation context's write-side
// repository the subscribe gate uses. Exported so internal/fx can bridge a
// concrete adapter to it without this package needing to know the
// adapter's concrete type. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type MemberChecker interface {
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domain.Member, error)
}

// WSHandler upgrades /ws connections, authenticates them via the
// Sec-WebSocket-Protocol subprotocol, and runs the subscribe/unsubscribe
// control protocol against the Hub.
type WSHandler struct {
	hub            *Hub
	verifier       *infraauth.JWTVerifier
	members        MemberChecker
	originPatterns []string
	bufSize        int
	presence       PresenceRecorder
}

// NewWSHandler builds the handler. originPatterns is passed straight to
// websocket.AcceptOptions.OriginPatterns; a nil/empty slice keeps
// coder/websocket's same-origin default.
func NewWSHandler(hub *Hub, verifier *infraauth.JWTVerifier, members MemberChecker, originPatterns []string, presence PresenceRecorder) *WSHandler {
	return &WSHandler{
		hub:            hub,
		verifier:       verifier,
		members:        members,
		originPatterns: originPatterns,
		bufSize:        defaultConnBuffer,
		presence:       presence,
	}
}

func (h *WSHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/ws", h.Handle)
}

func (h *WSHandler) Handle(c *gin.Context) {
	token, ok := bearerToken(c.Request.Header.Get("Sec-WebSocket-Protocol"))
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	userID, expiresAt, err := h.verifier.VerifyWithExpiry(token)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	sock, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		Subprotocols:   []string{"bearer"},
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		return // Accept wrote the failure response
	}
	defer sock.CloseNow()

	log.Printf("ws: connected user=%s", userID)
	defer log.Printf("ws: disconnected user=%s", userID)

	ctx := c.Request.Context()

	conn := newConn(userID, h.bufSize)
	h.hub.Register(conn)
	if err := h.presence.Connected(ctx, userID); err != nil {
		log.Printf("ws: presence Connected user=%s: %v", userID, err)
	}
	defer func() {
		h.hub.Remove(conn)
		if h.hub.UserConnCount(userID) == 0 {
			if err := h.presence.Disconnected(context.Background(), userID); err != nil {
				log.Printf("ws: presence Disconnected user=%s: %v", userID, err)
			}
		}
	}()

	go h.writePump(ctx, sock, conn, expiresAt)

	for {
		typ, data, err := sock.Read(ctx)
		if err != nil {
			conn.close()
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		h.handleControl(ctx, conn, data)
	}
}

// writePump is the single writer for the socket: it drains conn.out, keeps the
// peer alive with a periodic ping (net/http clears the socket deadline on the
// WS hijack, so a silently-dropped peer would otherwise leak both goroutines,
// the Conn, and its Hub entries), and closes the socket once the access token's
// exp passes.
func (h *WSHandler) writePump(ctx context.Context, sock *websocket.Conn, conn *Conn, expiresAt time.Time) {
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	expiry := time.NewTimer(time.Until(expiresAt))
	defer expiry.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.closed:
			sock.Close(websocket.StatusPolicyViolation, "client too slow")
			return
		case <-expiry.C:
			sock.Close(websocket.StatusPolicyViolation, "token expired")
			return
		case <-pingTicker.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := sock.Ping(pctx)
			cancel()
			if err != nil {
				conn.close()
				return
			}
			if err := h.presence.Heartbeat(context.Background(), conn.userID); err != nil {
				log.Printf("ws: presence Heartbeat user=%s: %v", conn.userID, err)
			}
		case payload := <-conn.out:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := sock.Write(wctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				conn.close()
				return
			}
		}
	}
}

type clientFrame struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
}

func (h *WSHandler) handleControl(ctx context.Context, conn *Conn, data []byte) {
	var f clientFrame
	if err := json.Unmarshal(data, &f); err != nil {
		conn.enqueue(serverError("", "malformed frame"))
		return
	}
	cid, err := uuid.Parse(f.ConversationID)
	if err != nil {
		conn.enqueue(serverError(f.ConversationID, "invalid conversation_id"))
		return
	}
	switch f.Type {
	case "subscribe":
		if h.hub.IsSubscribed(conn, cid) {
			// a re-subscribe skips the membership round trip
			conn.enqueue(serverAck("subscribed", cid.String()))
			return
		}
		if _, err := h.members.GetMember(ctx, cid, conn.userID); err != nil {
			if errors.Is(err, domain.ErrNotMember) {
				conn.enqueue(serverError(cid.String(), "not a member"))
			} else {
				log.Printf("ws: GetMember user=%s conv=%s: %v", conn.userID, cid, err)
				conn.enqueue(serverError(cid.String(), "internal error"))
			}
			return
		}
		h.hub.Subscribe(conn, cid)
		// ack after Subscribe so a racing broadcast is delivered, not dropped in a gap
		conn.enqueue(serverAck("subscribed", cid.String()))
		log.Printf("ws: user=%s %s conversation=%s", conn.userID, f.Type, cid)
	case "unsubscribe":
		h.hub.Unsubscribe(conn, cid)
		conn.enqueue(serverAck("unsubscribed", cid.String()))
		log.Printf("ws: user=%s %s conversation=%s", conn.userID, f.Type, cid)
	default:
		conn.enqueue(serverError(cid.String(), "unknown frame type"))
	}
}

// bearerToken parses a `Sec-WebSocket-Protocol: bearer, <jwt>` header value. It
// expects exactly two comma-separated tokens — the literal `bearer` and the JWT
// — and the scheme match is case-sensitive (RFC 6455 subprotocol tokens are
// case-sensitive).
func bearerToken(header string) (string, bool) {
	parts := strings.Split(header, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != "bearer" {
		return "", false
	}
	tok := strings.TrimSpace(parts[1])
	if tok == "" {
		return "", false
	}
	return tok, true
}

func serverAck(kind, conversationID string) []byte {
	b, _ := json.Marshal(map[string]string{"type": kind, "conversation_id": conversationID})
	return b
}

func serverError(conversationID, msg string) []byte {
	b, _ := json.Marshal(map[string]string{"type": "error", "conversation_id": conversationID, "error": msg})
	return b
}
