// internal/conversation/adapter/out/messaging/event_publisher_test.go
package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/conversation/domain"
)

func TestEventPublisher_PublishesOnConversationSubject(t *testing.T) {
	pub := NewEventPublisher(testJS)

	convID := uuid.Must(uuid.NewV7())
	received := make(chan []byte, 1)
	sub, err := testNC.Subscribe(WildcardSubject, func(m *natsgo.Msg) { received <- m.Data })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	ev := domain.MemberAdded{ConversationID: convID, UserID: uuid.Must(uuid.NewV7()), Kind: domain.KindGroupChat}
	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case data := <-received:
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.Type != "member.added" || env.ConversationID != convID.String() {
			t.Fatalf("got envelope %+v", env)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for published event")
	}
}
