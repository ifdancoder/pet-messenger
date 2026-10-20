// internal/notification/adapter/out/messaging/producer_test.go
package messaging

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

var (
	testNC *natsgo.Conn
	testJS natsgo.JetStreamContext
)

const notifStream = "NOTIF"

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		log.Fatalf("start nats container: %v", err)
	}
	url, err := container.ConnectionString(ctx)
	if err != nil {
		log.Fatalf("nats connection string: %v", err)
	}

	testNC, err = connectWithRetry(url)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	testJS, err = testNC.JetStream()
	if err != nil {
		log.Fatalf("jetstream context: %v", err)
	}
	if _, err := testJS.AddStream(&natsgo.StreamConfig{
		Name:      notifStream,
		Subjects:  []string{"notif.msg.*", "notif.member.*", "notif.call.*"},
		Retention: natsgo.WorkQueuePolicy,
		Storage:   natsgo.FileStorage,
		MaxAge:    24 * time.Hour,
		Discard:   natsgo.DiscardOld,
	}); err != nil {
		log.Fatalf("add NOTIF stream: %v", err)
	}

	code := m.Run()

	testNC.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func connectWithRetry(url string) (*natsgo.Conn, error) {
	var lastErr error
	for range 20 {
		nc, err := natsgo.Connect(url)
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return nil, lastErr
}

func startJetStream(t *testing.T) natsgo.JetStreamContext {
	t.Helper()
	if err := testJS.PurgeStream(notifStream); err != nil {
		t.Fatalf("purge %s stream: %v", notifStream, err)
	}
	return testJS
}

func TestProducer_MessageSent_GroupChat(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js, testNC)

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
	p := NewProducer(js, testNC)

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
	p := NewProducer(js, testNC)

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
	p := NewProducer(js, testNC)

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

func TestProducer_CallStarted(t *testing.T) {
	js := startJetStream(t)
	p := NewProducer(js, testNC)

	callID := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	initiator := uuid.Must(uuid.NewV7())

	if err := p.CallStarted(context.Background(), callID, convID, initiator); err != nil {
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

func TestProducer_Notify_PublishesPerUserSubject(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())

	got := make(chan *natsgo.Msg, 1)
	sub, err := testNC.Subscribe("notif.ws."+uid.String(), func(m *natsgo.Msg) { got <- m })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	if err := testNC.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	p := NewProducer(testJS, testNC)
	payload := []byte(`{"hello":"world"}`)
	if err := p.Notify(uid, payload); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case m := <-got:
		if string(m.Data) != string(payload) {
			t.Errorf("payload = %q, want %q", m.Data, payload)
		}
		if m.Subject != "notif.ws."+uid.String() {
			t.Errorf("subject = %q", m.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received on notif.ws.<uid>")
	}

	if err := p.Notify(uuid.Must(uuid.NewV7()), []byte(`{}`)); err != nil {
		t.Fatalf("Notify other: %v", err)
	}
	select {
	case m := <-got:
		t.Fatalf("unexpected message for another user: %q on %s", m.Data, m.Subject)
	case <-time.After(300 * time.Millisecond):
	}
}
