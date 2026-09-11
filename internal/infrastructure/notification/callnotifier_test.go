package notification

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCallNotifier_CallStarted(t *testing.T) {
	js := startJetStream(t)
	n := NewCallNotifier(js)

	callID := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	initiator := uuid.Must(uuid.NewV7())

	if err := n.CallStarted(context.Background(), callID, convID, initiator); err != nil {
		t.Fatalf("CallStarted: %v", err)
	}
	raw, err := js.GetLastMsg(notifStream, "notif.call."+callID.String())
	if err != nil {
		t.Fatalf("GetLastMsg: %v", err)
	}
	var got NotifIntent
	if err := json.Unmarshal(raw.Data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != "call_started" || got.ConversationID != convID.String() || got.CallID != callID.String() {
		t.Fatalf("intent = %+v", got)
	}
	if got.SenderID == nil || *got.SenderID != initiator.String() || got.DedupPrefix != "call" {
		t.Fatalf("sender/dedup = %v / %q", got.SenderID, got.DedupPrefix)
	}
}
