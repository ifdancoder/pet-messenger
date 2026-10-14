// internal/notification/adapter/in/messaging/worker_test.go
package messaging

import (
	"context"
	"errors"
	"testing"

	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/notification/application/port/in"
)

type fakeProcess struct {
	handle func(cmd in.ProcessNotificationIntentCommand) error
	calls  []in.ProcessNotificationIntentCommand
}

func (f *fakeProcess) Handle(_ context.Context, cmd in.ProcessNotificationIntentCommand) error {
	f.calls = append(f.calls, cmd)
	return f.handle(cmd)
}

// *natsgo.Msg is a plain struct (nats.go v1.53.1, nats.go:797-808); no
// live connection is needed to construct one. Calling Ack()/Nak()/Term() on
// a Msg with a nil Sub safely returns nats.ErrMsgNotBound (checked early in
// (*Msg).checkReply, nats.go js.go:3579-3586) rather than panicking, so
// HandleMsg's discarded `_ = m.Ack()` etc. are safe to call here with no
// special setup beyond Data. The real end-to-end Ack/Nak proof against a
// live JetStream consumer is Task 13's integration test.
func TestWorker_DecodesAndCallsProcess(t *testing.T) {
	fp := &fakeProcess{handle: func(in.ProcessNotificationIntentCommand) error { return nil }}
	w := NewWorker(fp)

	data := []byte(`{"kind":"message","conversation_id":"c1","message_id":"m1","dedup_prefix":"msg"}`)
	w.HandleMsg(&natsgo.Msg{Data: data})

	if len(fp.calls) != 1 {
		t.Fatalf("process called %d times, want 1", len(fp.calls))
	}
	got := fp.calls[0]
	if got.Kind != "message" || got.ConversationID != "c1" || got.MessageID == nil || *got.MessageID != "m1" || got.DedupPrefix != "msg" {
		t.Errorf("cmd = %+v", got)
	}
}

func TestWorker_BadJSON_Terminated(t *testing.T) {
	fp := &fakeProcess{handle: func(in.ProcessNotificationIntentCommand) error {
		t.Fatal("process must not be called on undecodable JSON")
		return nil
	}}
	w := NewWorker(fp)
	w.HandleMsg(&natsgo.Msg{Data: []byte("{not json")})
	if len(fp.calls) != 0 {
		t.Fatalf("process called %d times, want 0", len(fp.calls))
	}
}

func TestWorker_ProcessError_StillReturns(t *testing.T) {
	fp := &fakeProcess{handle: func(in.ProcessNotificationIntentCommand) error { return errors.New("boom") }}
	w := NewWorker(fp)
	data := []byte(`{"kind":"call_started","conversation_id":"c1","call_id":"call1","dedup_prefix":"call"}`)
	w.HandleMsg(&natsgo.Msg{Data: data}) // must not panic; Ack/Nak against an unbound msg just errors internally, discarded
	if len(fp.calls) != 1 {
		t.Fatalf("process called %d times, want 1", len(fp.calls))
	}
}
