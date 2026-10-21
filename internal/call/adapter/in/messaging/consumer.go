// Package messaging consumes the media-service's core-NATS room events on
// every api pod: call.<id>.participant_left and call.<id>.ended. Handlers
// are idempotent (the HandleParticipantLeft/HandleCallEnded commands
// tolerate repeats), so running once per pod is harmless.
package messaging

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	in "pet-messenger/internal/call/application/port/in"
)

// Wildcard is the core-NATS subject this consumer subscribes to.
const Wildcard = "call.>"

// Consumer dispatches call.<id>.<event> messages to the matching command.
type Consumer struct {
	ended in.HandleCallEndedUseCase
	left  in.HandleParticipantLeftUseCase
}

// NewConsumer wires the Consumer.
func NewConsumer(ended in.HandleCallEndedUseCase, left in.HandleParticipantLeftUseCase) *Consumer {
	return &Consumer{ended: ended, left: left}
}

// HandleMsg parses "call.<uuid>.<event>" and dispatches. Payloads:
// participant_left: {"user_id":"<uuid>"}; ended: {}.
func (c *Consumer) HandleMsg(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	parts := strings.Split(m.Subject, ".")
	if len(parts) != 3 || parts[0] != "call" {
		log.Printf("callmessaging: bad subject %q", m.Subject)
		return
	}
	callID, err := uuid.Parse(parts[1])
	if err != nil {
		log.Printf("callmessaging: bad call id in %q", m.Subject)
		return
	}
	switch parts[2] {
	case "ended":
		if err := c.ended.Handle(ctx, in.HandleCallEndedCommand{CallID: callID}); err != nil {
			log.Printf("callmessaging: HandleCallEnded %s: %v", callID, err)
		}
	case "participant_left":
		var body struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(m.Data, &body); err != nil {
			log.Printf("callmessaging: bad participant_left payload: %v", err)
			return
		}
		uid, err := uuid.Parse(body.UserID)
		if err != nil {
			log.Printf("callmessaging: bad user id %q", body.UserID)
			return
		}
		if err := c.left.Handle(ctx, in.HandleParticipantLeftCommand{CallID: callID, UserID: uid}); err != nil {
			log.Printf("callmessaging: HandleParticipantLeft %s/%s: %v", callID, uid, err)
		}
	case "participant_joined":
		// Informational only; the join endpoint already wrote the row.
	default:
		log.Printf("callmessaging: unknown event %q", parts[2])
	}
}
