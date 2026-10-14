package notification

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	domainmsg "pet-messenger/internal/message/domain"

	domainconv "pet-messenger/internal/conversation/domain"
)

// startJetStream returns the package-shared JetStream context with the NOTIF
// stream purged, so each test starts from an empty work queue.
func startJetStream(t *testing.T) natsgo.JetStreamContext {
	t.Helper()
	if err := testJS.PurgeStream(notifStream); err != nil {
		t.Fatalf("purge %s stream: %v", notifStream, err)
	}
	return testJS
}

func TestProducer_MessageSent_GroupChat(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js)

	sender := uuid.Must(uuid.NewV7())
	m := &domainmsg.Message{
		ID:       uuid.Must(uuid.NewV7()),
		Seq:      42,
		SenderID: &sender,
		Type:     domainmsg.TypeText,
		Content:  "hello there",
	}
	conv := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}

	if err := p.MessageSent(context.Background(), m, conv); err != nil {
		t.Fatalf("MessageSent: %v", err)
	}

	raw, err := js.GetLastMsg("NOTIF", "notif.msg."+m.ID.String())
	if err != nil {
		t.Fatalf("GetLastMsg: %v", err)
	}
	var got NotifIntent
	if err := json.Unmarshal(raw.Data, &got); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if got.Kind != "message" || got.ConversationID != conv.ID.String() {
		t.Errorf("intent = %+v", got)
	}
	if got.MessageID == nil || *got.MessageID != m.ID.String() {
		t.Errorf("MessageID = %v", got.MessageID)
	}
	if got.MessageSeq == nil || *got.MessageSeq != 42 {
		t.Errorf("MessageSeq = %v", got.MessageSeq)
	}
	if got.SenderID == nil || *got.SenderID != sender.String() {
		t.Errorf("SenderID = %v", got.SenderID)
	}
	if got.Preview != "hello there" || got.DedupPrefix != "msg" {
		t.Errorf("preview/dedup = %q / %q", got.Preview, got.DedupPrefix)
	}
}

func TestProducer_MessageSent_ChannelIsNoOp(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js)

	m := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), Seq: 1, Type: domainmsg.TypeText, Content: "x"}
	conv := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelBroadcast}

	if err := p.MessageSent(context.Background(), m, conv); err != nil {
		t.Fatalf("MessageSent: %v", err)
	}
	info, err := js.StreamInfo("NOTIF")
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Fatalf("channel message published %d intents, want 0", info.State.Msgs)
	}
}

func TestProducer_MembershipEvent_TierChanged(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js)

	convID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())

	if err := p.MembershipEvent(context.Background(), domainmsg.SystemMemberTierChanged, convID, target, "admin"); err != nil {
		t.Fatalf("MembershipEvent: %v", err)
	}

	raw, err := js.GetLastMsg("NOTIF", "notif.member.*")
	if err != nil {
		t.Fatalf("GetLastMsg: %v", err)
	}
	var got NotifIntent
	if err := json.Unmarshal(raw.Data, &got); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if got.Kind != "member_tier_changed" || got.ConversationID != convID.String() {
		t.Errorf("intent = %+v", got)
	}
	if got.RecipientID == nil || *got.RecipientID != target.String() {
		t.Errorf("RecipientID = %v", got.RecipientID)
	}
	if got.NewTier != "admin" || got.DedupPrefix != "tier" {
		t.Errorf("newTier/dedup = %q / %q", got.NewTier, got.DedupPrefix)
	}
}

func TestProducer_MembershipEvent_UnrelatedIsNoOp(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js)

	if err := p.MembershipEvent(context.Background(), domainmsg.SystemMemberJoined, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), ""); err != nil {
		t.Fatalf("MembershipEvent: %v", err)
	}
	info, err := js.StreamInfo("NOTIF")
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Fatalf("member_joined published %d intents, want 0", info.State.Msgs)
	}
}
