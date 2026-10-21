// internal/message/adapter/out/messaging/publisher_test.go
package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	natsinfra "pet-messenger/internal/platform/infrastructure/nats"
	domainmsg "pet-messenger/internal/message/domain"
)

func TestPublisher_PublishCreated(t *testing.T) {
	pub := NewPublisher(testNC)
	m, err := domainmsg.NewText(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "hi", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	m.Seq = 3

	received := make(chan []byte, 1)
	sub, err := testNC.Subscribe(natsinfra.SubjectForConversation(m.ConversationID.String()), func(msg *natsgo.Msg) { received <- msg.Data })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	if err := pub.PublishCreated(context.Background(), m); err != nil {
		t.Fatalf("PublishCreated: %v", err)
	}

	select {
	case data := <-received:
		var env natsinfra.MessageEvent
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if env.Type != natsinfra.MessageCreated || env.ConversationID != m.ConversationID.String() || env.Message.ID != m.ID.String() || env.Message.Seq != 3 {
			t.Fatalf("got envelope %+v", env)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestPublisher_PublishUpdatedAndDeleted(t *testing.T) {
	pub := NewPublisher(testNC)
	m, err := domainmsg.NewText(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "hi", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}

	received := make(chan []byte, 2)
	sub, err := testNC.Subscribe(natsinfra.SubjectForConversation(m.ConversationID.String()), func(msg *natsgo.Msg) { received <- msg.Data })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	if err := pub.PublishUpdated(context.Background(), m); err != nil {
		t.Fatalf("PublishUpdated: %v", err)
	}
	if err := pub.PublishDeleted(context.Background(), m); err != nil {
		t.Fatalf("PublishDeleted: %v", err)
	}

	wantTypes := []natsinfra.MessageEventType{natsinfra.MessageUpdated, natsinfra.MessageDeleted}
	for _, want := range wantTypes {
		select {
		case data := <-received:
			var env natsinfra.MessageEvent
			if err := json.Unmarshal(data, &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if env.Type != want {
				t.Fatalf("got type %q, want %q", env.Type, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %q event", want)
		}
	}
}
