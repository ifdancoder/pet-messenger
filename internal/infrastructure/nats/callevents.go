package nats

import (
	"encoding/json"
	"log"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
)

// CallEventPublisher emits the media-service's room events on core NATS
// (call.<id>.<event>), consumed by every api pod's call context messaging
// Consumer (internal/call/adapter/in/messaging). Best-effort: a publish
// failure is logged, nothing blocks the room.
type CallEventPublisher struct{ nc *natsgo.Conn }

func NewCallEventPublisher(nc *natsgo.Conn) *CallEventPublisher { return &CallEventPublisher{nc: nc} }

func (p *CallEventPublisher) ParticipantJoined(callID, userID uuid.UUID, role string) {
	p.pub(callID, "participant_joined", map[string]string{"user_id": userID.String(), "role": role})
}

func (p *CallEventPublisher) ParticipantLeft(callID, userID uuid.UUID) {
	p.pub(callID, "participant_left", map[string]string{"user_id": userID.String()})
}

func (p *CallEventPublisher) Ended(callID uuid.UUID) {
	p.pub(callID, "ended", struct{}{})
}

func (p *CallEventPublisher) pub(callID uuid.UUID, event string, payload any) {
	b, _ := json.Marshal(payload)
	if err := p.nc.Publish("call."+callID.String()+"."+event, b); err != nil {
		log.Printf("nats: publish call.%s.%s: %v", callID, event, err)
	}
}
