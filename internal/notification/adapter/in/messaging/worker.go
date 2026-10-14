// internal/notification/adapter/in/messaging/worker.go

// Package messaging is the notification context's primary ("driving")
// JetStream adapter: it decodes a published NotifIntent (see
// adapter/out/messaging.NotifIntent, the same bounded context's producer)
// and drives the ProcessNotificationIntent use case. All business logic
// (recipient resolution, presence/push fallback, dedup) lives in that
// command, not here.
package messaging

import (
	"context"
	"encoding/json"
	"log"
	"time"

	natsgo "github.com/nats-io/nats.go"

	outmessaging "pet-messenger/internal/notification/adapter/out/messaging"
	"pet-messenger/internal/notification/application/port/in"
)

// Worker is the JetStream queue-subscribe callback owner.
type Worker struct {
	process in.ProcessNotificationIntentUseCase
}

// NewWorker wires the delivery worker to the ProcessNotificationIntent use
// case.
func NewWorker(process in.ProcessNotificationIntentUseCase) *Worker {
	return &Worker{process: process}
}

// HandleMsg is the JetStream queue-subscribe callback. It never returns an
// error; it Acks / Naks / Terms the message itself.
func (w *Worker) HandleMsg(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var intent outmessaging.NotifIntent
	if err := json.Unmarshal(m.Data, &intent); err != nil {
		log.Printf("notification: undecodable intent on %s: %v", m.Subject, err)
		_ = m.Term()
		return
	}

	cmd := in.ProcessNotificationIntentCommand{
		Kind: intent.Kind, ConversationID: intent.ConversationID, MessageID: intent.MessageID,
		MessageSeq: intent.MessageSeq, SenderID: intent.SenderID, Preview: intent.Preview,
		RecipientID: intent.RecipientID, NewTier: intent.NewTier, CallID: intent.CallID,
		DedupPrefix: intent.DedupPrefix,
	}
	if err := w.process.Handle(ctx, cmd); err != nil {
		log.Printf("notification: process intent on %s: %v; will retry", m.Subject, err)
		_ = m.Nak()
		return
	}
	_ = m.Ack()
}
