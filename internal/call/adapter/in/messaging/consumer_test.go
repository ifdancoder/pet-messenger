package messaging

import (
	"context"
	"testing"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	in "pet-messenger/internal/call/application/port/in"
)

type fakeHandleCallEnded struct {
	called []uuid.UUID
}

func (f *fakeHandleCallEnded) Handle(_ context.Context, cmd in.HandleCallEndedCommand) error {
	f.called = append(f.called, cmd.CallID)
	return nil
}

type fakeHandleParticipantLeft struct {
	called []in.HandleParticipantLeftCommand
}

func (f *fakeHandleParticipantLeft) Handle(_ context.Context, cmd in.HandleParticipantLeftCommand) error {
	f.called = append(f.called, cmd)
	return nil
}

func TestConsumer_Ended_CallsHandleCallEnded(t *testing.T) {
	ended := &fakeHandleCallEnded{}
	cons := NewConsumer(ended, &fakeHandleParticipantLeft{})
	callID := uuid.Must(uuid.NewV7())

	cons.HandleMsg(&natsgo.Msg{Subject: "call." + callID.String() + ".ended", Data: []byte(`{}`)})

	if len(ended.called) != 1 || ended.called[0] != callID {
		t.Fatalf("ended.called = %v, want [%v]", ended.called, callID)
	}
}

func TestConsumer_Ended_DispatchesEveryDelivery(t *testing.T) {
	// The consumer itself dispatches every delivery unconditionally;
	// idempotency (ending the same call twice is a no-op) is the
	// HandleCallEnded command's own responsibility, already covered by
	// TestHandleCallEnded_Idempotent in the command package.
	ended := &fakeHandleCallEnded{}
	cons := NewConsumer(ended, &fakeHandleParticipantLeft{})
	callID := uuid.Must(uuid.NewV7())
	subj := "call." + callID.String() + ".ended"

	cons.HandleMsg(&natsgo.Msg{Subject: subj, Data: []byte(`{}`)})
	cons.HandleMsg(&natsgo.Msg{Subject: subj, Data: []byte(`{}`)})

	if len(ended.called) != 2 {
		t.Fatalf("ended.called = %d, want 2", len(ended.called))
	}
}

func TestConsumer_ParticipantLeft_CallsHandleParticipantLeft(t *testing.T) {
	left := &fakeHandleParticipantLeft{}
	cons := NewConsumer(&fakeHandleCallEnded{}, left)
	callID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	cons.HandleMsg(&natsgo.Msg{
		Subject: "call." + callID.String() + ".participant_left",
		Data:    []byte(`{"user_id":"` + userID.String() + `"}`),
	})

	if len(left.called) != 1 || left.called[0].CallID != callID || left.called[0].UserID != userID {
		t.Fatalf("left.called = %+v, want one entry for %v/%v", left.called, callID, userID)
	}
}

func TestConsumer_BadSubject_NoPanic(t *testing.T) {
	cons := NewConsumer(&fakeHandleCallEnded{}, &fakeHandleParticipantLeft{})
	cons.HandleMsg(&natsgo.Msg{Subject: "not-a-call-subject"})
	cons.HandleMsg(&natsgo.Msg{Subject: "call.not-a-uuid.ended"})
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + uuid.Must(uuid.NewV7()).String() + ".weird"})
}

func TestConsumer_ParticipantLeft_BadPayload_NoPanic(t *testing.T) {
	cons := NewConsumer(&fakeHandleCallEnded{}, &fakeHandleParticipantLeft{})
	id := uuid.Must(uuid.NewV7()).String()
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + id + ".participant_left", Data: []byte(`not json`)})
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + id + ".participant_left", Data: []byte(`{"user_id":"nope"}`)})
}
