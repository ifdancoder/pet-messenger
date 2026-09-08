package ws

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	infraauth "pet-messenger/internal/infrastructure/auth"
)

// stubConvRepo implements conversation.Repository; only GetMember is meaningful.
type stubConvRepo struct{ memberErr error }

func (s stubConvRepo) Create(context.Context, *conversation.Conversation, []*conversation.Member) error {
	return nil
}
func (s stubConvRepo) GetByID(context.Context, uuid.UUID) (*conversation.Conversation, error) {
	return nil, conversation.ErrNotFound
}
func (s stubConvRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	if s.memberErr != nil {
		return nil, s.memberErr
	}
	return &conversation.Member{Role: conversation.RoleMember}, nil
}
func (s stubConvRepo) GetDirect(context.Context, uuid.UUID, uuid.UUID) (*conversation.Conversation, error) {
	return nil, conversation.ErrNotFound
}
func (s stubConvRepo) ListForUser(context.Context, uuid.UUID, time.Time, uuid.UUID, int) ([]*conversation.Conversation, error) {
	return nil, nil
}

func wsTestServer(t *testing.T, members conversation.Repository) (*httptest.Server, *infraauth.JWTIssuer, *Hub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	issuer := infraauth.NewJWTIssuerWithKey(priv, time.Minute)
	verifier := infraauth.NewJWTVerifierWithKey(pub)
	hub := NewHub()
	h := NewWSHandler(hub, verifier, members)
	r := gin.New()
	h.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, issuer, hub
}

func dialWS(t *testing.T, srv *httptest.Server, protocols ...string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: protocols})
}

func readFrame(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	return m
}

func TestWSHandler_RejectsMissingToken(t *testing.T) {
	srv, _, _ := wsTestServer(t, stubConvRepo{})
	if _, _, err := dialWS(t, srv); err == nil {
		t.Fatal("dial without a subprotocol should fail")
	}
}

func TestWSHandler_RejectsBadToken(t *testing.T) {
	srv, _, _ := wsTestServer(t, stubConvRepo{})
	if _, _, err := dialWS(t, srv, "bearer", "not-a-jwt"); err == nil {
		t.Fatal("dial with a bad token should fail")
	}
}

func TestWSHandler_AcceptsValidToken(t *testing.T) {
	srv, issuer, _ := wsTestServer(t, stubConvRepo{})
	tok, _, _ := issuer.IssueAccessToken(uuid.Must(uuid.NewV7()))
	c, _, err := dialWS(t, srv, "bearer", tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	if c.Subprotocol() != "bearer" {
		t.Fatalf("negotiated subprotocol = %q", c.Subprotocol())
	}
}

func TestWSHandler_SubscribeRequiresMembership(t *testing.T) {
	srv, issuer, _ := wsTestServer(t, stubConvRepo{memberErr: conversation.ErrNotMember})
	tok, _, _ := issuer.IssueAccessToken(uuid.Must(uuid.NewV7()))
	c, _, err := dialWS(t, srv, "bearer", tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	cid := uuid.Must(uuid.NewV7())
	writeJSON(t, c, clientFrame{Type: "subscribe", ConversationID: cid.String()})
	frame := readFrame(t, c)
	if frame["type"] != "error" || frame["error"] != "not a member" {
		t.Fatalf("frame = %v", frame)
	}
}

func TestWSHandler_SubscribeAckThenDelivery(t *testing.T) {
	srv, issuer, hub := wsTestServer(t, stubConvRepo{})
	tok, _, _ := issuer.IssueAccessToken(uuid.Must(uuid.NewV7()))
	c, _, err := dialWS(t, srv, "bearer", tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	cid := uuid.Must(uuid.NewV7())
	writeJSON(t, c, clientFrame{Type: "subscribe", ConversationID: cid.String()})
	if ack := readFrame(t, c); ack["type"] != "subscribed" {
		t.Fatalf("ack = %v", ack)
	}
	// give the server a beat to register the subscription, then broadcast
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.Broadcast(cid, []byte(`{"type":"message.created","conversation_id":"`+cid.String()+`","message":{"content":"live"}}`))
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_, data, err := c.Read(ctx)
		cancel()
		if err == nil {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			if m["type"] == "message.created" {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("did not receive the broadcast")
		}
	}
}

func TestWSHandler_MalformedFrame(t *testing.T) {
	srv, issuer, _ := wsTestServer(t, stubConvRepo{})
	tok, _, _ := issuer.IssueAccessToken(uuid.Must(uuid.NewV7()))
	c, _, err := dialWS(t, srv, "bearer", tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte("not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if frame := readFrame(t, c); frame["type"] != "error" || frame["error"] != "malformed frame" {
		t.Fatalf("frame = %v", frame)
	}
}

func TestWSHandler_DisconnectRemovesFromHub(t *testing.T) {
	srv, issuer, hub := wsTestServer(t, stubConvRepo{})
	tok, _, _ := issuer.IssueAccessToken(uuid.Must(uuid.NewV7()))
	c, _, err := dialWS(t, srv, "bearer", tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cid := uuid.Must(uuid.NewV7())
	writeJSON(t, c, clientFrame{Type: "subscribe", ConversationID: cid.String()})
	_ = readFrame(t, c) // ack
	c.CloseNow()

	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.RLock()
		n := len(hub.subs[cid])
		hub.mu.RUnlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("hub still has %d subscribers for the conversation after disconnect", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func writeJSON(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}
