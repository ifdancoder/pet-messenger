package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	infraauth "pet-messenger/internal/infrastructure/auth"
)

// memberChecker is the slice of conversation.Repository the subscribe gate uses.
type memberChecker interface {
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error)
}

// WSHandler upgrades /ws connections, authenticates them via the
// Sec-WebSocket-Protocol subprotocol, and runs the subscribe/unsubscribe
// control protocol against the Hub.
type WSHandler struct {
	hub      *Hub
	verifier *infraauth.JWTVerifier
	members  memberChecker
	bufSize  int
}

func NewWSHandler(hub *Hub, verifier *infraauth.JWTVerifier, members conversation.Repository) *WSHandler {
	return &WSHandler{hub: hub, verifier: verifier, members: members, bufSize: 64}
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
	userID, err := h.verifier.Verify(token)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	sock, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		Subprotocols: []string{"bearer"},
	})
	if err != nil {
		return // Accept wrote the failure response
	}
	defer sock.CloseNow()

	conn := newConn(userID, h.bufSize)
	defer h.hub.Remove(conn)

	ctx := c.Request.Context()
	go h.writePump(ctx, sock, conn)

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

func (h *WSHandler) writePump(ctx context.Context, sock *websocket.Conn, conn *Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.closed:
			sock.Close(websocket.StatusPolicyViolation, "client too slow")
			return
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
		if _, err := h.members.GetMember(ctx, cid, conn.userID); err != nil {
			conn.enqueue(serverError(f.ConversationID, "not a member"))
			return
		}
		h.hub.Subscribe(conn, cid)
		conn.enqueue(serverAck("subscribed", f.ConversationID))
	case "unsubscribe":
		h.hub.Unsubscribe(conn, cid)
		conn.enqueue(serverAck("unsubscribed", f.ConversationID))
	default:
		conn.enqueue(serverError(f.ConversationID, "unknown frame type"))
	}
}

// bearerToken parses a `Sec-WebSocket-Protocol: bearer, <jwt>` header value.
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
