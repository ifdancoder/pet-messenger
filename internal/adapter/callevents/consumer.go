// Package callevents consumes the media-service's core-NATS room events on
// every api pod: call.<id>.participant_left and call.<id>.ended. Handlers are
// idempotent (usecase HandleParticipantLeft / HandleCallEnded tolerate
// repeats), so running once per pod is harmless.
package callevents

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	usecasecall "pet-messenger/internal/usecase/call"
)

const Wildcard = "call.>"

type Consumer struct{ svc *usecasecall.Service }

func NewConsumer(svc *usecasecall.Service) *Consumer { return &Consumer{svc: svc} }

// HandleMsg parses "call.<uuid>.<event>" and dispatches. Payloads:
// participant_left: {"user_id":"<uuid>"}; ended: {} .
func (c *Consumer) HandleMsg(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	parts := strings.Split(m.Subject, ".")
	if len(parts) != 3 || parts[0] != "call" {
		log.Printf("callevents: bad subject %q", m.Subject)
		return
	}
	callID, err := uuid.Parse(parts[1])
	if err != nil {
		log.Printf("callevents: bad call id in %q", m.Subject)
		return
	}
	switch parts[2] {
	case "ended":
		if err := c.svc.HandleCallEnded(ctx, callID); err != nil {
			log.Printf("callevents: HandleCallEnded %s: %v", callID, err)
		}
	case "participant_left":
		var body struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(m.Data, &body); err != nil {
			log.Printf("callevents: bad participant_left payload: %v", err)
			return
		}
		uid, err := uuid.Parse(body.UserID)
		if err != nil {
			log.Printf("callevents: bad user id %q", body.UserID)
			return
		}
		if err := c.svc.HandleParticipantLeft(ctx, callID, uid); err != nil {
			log.Printf("callevents: HandleParticipantLeft %s/%s: %v", callID, uid, err)
		}
	case "participant_joined":
		// informational only; the join endpoint already wrote the row.
	default:
		log.Printf("callevents: unknown event %q", parts[2])
	}
}
