package ws

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"

	"pet-messenger/internal/domain/message"
	infraauth "pet-messenger/internal/infrastructure/auth"
	natsinfra "pet-messenger/internal/infrastructure/nats"

	convmsg "pet-messenger/internal/conversation/adapter/out/messaging"
	convdomain "pet-messenger/internal/conversation/domain"
)

// TestCrossPodDelivery: a message published on one NATS connection ("pod A")
// reaches a WebSocket client connected to a different pod ("pod B") that has
// subscribed to the conversation.
func TestCrossPodDelivery(t *testing.T) {
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("start nats container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	url, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// pod B: hub + msg.> subscription + WS handler behind httptest
	ncB, err := connectTestNATS(url)
	if err != nil {
		t.Fatalf("pod B connect: %v", err)
	}
	t.Cleanup(func() { ncB.Close() })
	hubB := NewHub()
	subB, err := ncB.Subscribe(natsinfra.WildcardSubject, func(m *natsgo.Msg) {
		if cid, ok := natsinfra.ConversationIDFromSubject(m.Subject); ok {
			hubB.Broadcast(cid, m.Data)
		}
	})
	if err != nil {
		t.Fatalf("pod B subscribe: %v", err)
	}
	t.Cleanup(func() { _ = subB.Unsubscribe() })

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	issuer := newTestTokenIssuer(priv, time.Minute)
	verifier := infraauth.NewJWTVerifierWithKey(pub)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewWSHandler(hubB, verifier, stubConvRepo{}, nil, NopPresence{}).RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	userID := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	token, _, _ := issuer.IssueAccessToken(userID)

	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	cli, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws",
		&websocket.DialOptions{Subprotocols: []string{"bearer", token}})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer cli.CloseNow()

	writeJSON(t, cli, clientFrame{Type: "subscribe", ConversationID: convID.String()})
	if ack := readFrame(t, cli); ack["type"] != "subscribed" {
		t.Fatalf("ack = %v", ack)
	}

	// pod A publishes
	ncA, err := connectTestNATS(url)
	if err != nil {
		t.Fatalf("pod A connect: %v", err)
	}
	t.Cleanup(func() { ncA.Close() })
	publisher := natsinfra.NewPublisher(ncA)

	sender := uuid.Must(uuid.NewV7())
	m := &message.Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: convID,
		Seq:            1,
		SenderID:       &sender,
		Type:           message.TypeText,
		Content:        "cross-pod hello",
		CreatedAt:      time.Now().UTC(),
	}
	// retry the publish/read a few times to absorb subscription-propagation lag
	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := publisher.PublishCreated(ctx, m); err != nil {
			t.Fatalf("publish: %v", err)
		}
		readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_, data, err := cli.Read(readCtx)
		cancel()
		if err == nil {
			var ev struct {
				Type           string `json:"type"`
				ConversationID string `json:"conversation_id"`
				Message        struct {
					ID      string `json:"id"`
					Content string `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal(data, &ev); err != nil {
				t.Fatalf("unmarshal delivered frame %q: %v", data, err)
			}
			if ev.Type != "message.created" || ev.ConversationID != convID.String() ||
				ev.Message.ID != m.ID.String() || ev.Message.Content != "cross-pod hello" {
				t.Fatalf("delivered frame wrong: %s", data)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("WS client never received the cross-pod message")
		}
	}
}

// TestCrossPodMemberRevocation: a member.removed event published on one NATS
// connection ("pod A") makes a different pod ("pod B") drop that user's local
// Hub subscription to the conversation. This exercises the same
// conversation.events.> envelope + ephemeral JetStream subscription shape as
// internal/fx/conversation.go's startMemberRevocationFanout (the pre-retrofit
// member.revoked.> core-NATS mechanism this superseded — see the design
// spec's "Migration of existing wiring").
func TestCrossPodMemberRevocation(t *testing.T) {
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("start nats container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	url, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// pod B: Hub + an ephemeral conversation.events.> subscription that
	// calls hub.DropSubscription on a member.removed event.
	hub := NewHub()
	ncB, err := connectTestNATS(url)
	if err != nil {
		t.Fatalf("pod B connect: %v", err)
	}
	t.Cleanup(func() { ncB.Close() })
	jsB, err := ncB.JetStream()
	if err != nil {
		t.Fatalf("pod B jetstream: %v", err)
	}
	if err := convmsg.EnsureStream(jsB); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}
	subB, err := jsB.Subscribe(convmsg.WildcardSubject, func(m *natsgo.Msg) {
		var env struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(m.Data, &env); err != nil {
			t.Logf("pod B: unmarshal envelope %q: %v", m.Data, err)
			_ = m.Ack()
			return
		}
		if env.Type != "member.removed" {
			_ = m.Ack()
			return
		}
		var ev convdomain.MemberRemoved
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			t.Logf("pod B: unmarshal member.removed payload %q: %v", env.Payload, err)
			_ = m.Ack()
			return
		}
		hub.DropSubscription(ev.UserID, ev.ConversationID)
		_ = m.Ack()
	}, natsgo.ManualAck())
	if err != nil {
		t.Fatalf("pod B subscribe: %v", err)
	}
	t.Cleanup(func() { _ = subB.Unsubscribe() })

	conv := uuid.Must(uuid.NewV7())
	user := uuid.Must(uuid.NewV7())
	conn := newConn(user, 4)
	hub.Subscribe(conn, conv)

	// pod A publishes the member.removed event
	ncA, err := connectTestNATS(url)
	if err != nil {
		t.Fatalf("pod A connect: %v", err)
	}
	t.Cleanup(func() { ncA.Close() })
	jsA, err := ncA.JetStream()
	if err != nil {
		t.Fatalf("pod A jetstream: %v", err)
	}
	pub := convmsg.NewEventPublisher(jsA)

	// retry the publish to absorb subscription-propagation lag
	deadline := time.After(8 * time.Second)
	for {
		if err := pub.Publish(ctx, convdomain.MemberRemoved{ConversationID: conv, UserID: user}); err != nil {
			t.Fatalf("publish: %v", err)
		}
		if !hub.IsSubscribed(conn, conv) {
			return
		}
		select {
		case <-deadline:
			t.Fatal("pod B did not drop the subscription")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// The listening port can become ready before NATS completes startup.
func connectTestNATS(url string) (*natsgo.Conn, error) {
	var lastErr error
	for range 20 {
		nc, err := natsgo.Connect(url, natsgo.Timeout(time.Second))
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}
