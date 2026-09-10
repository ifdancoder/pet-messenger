package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
)

// CallNotifier publishes one notif.call.<callID> intent when a call starts, so
// the notification worker rings every other conversation member (online via WS,
// offline via device push). Best-effort: the caller logs and swallows errors.
type CallNotifier struct{ js natsgo.JetStreamContext }

func NewCallNotifier(js natsgo.JetStreamContext) *CallNotifier { return &CallNotifier{js: js} }

func (n *CallNotifier) CallStarted(ctx context.Context, callID, conversationID, initiatorID uuid.UUID) error {
	sid := initiatorID.String()
	data, err := json.Marshal(NotifIntent{
		Kind:           "call_started",
		ConversationID: conversationID.String(),
		SenderID:       &sid,
		CallID:         callID.String(),
		DedupPrefix:    "call",
	})
	if err != nil {
		return fmt.Errorf("marshal call notif intent: %w", err)
	}
	if _, err := n.js.Publish("notif.call."+callID.String(), data); err != nil {
		return fmt.Errorf("publish call notif intent: %w", err)
	}
	return nil
}
