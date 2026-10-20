// Package messaging holds the conversation context's event-driven
// secondary adapters: publishing this context's own domain events to NATS
// JetStream, and projecting them (plus the message domain's existing
// msg.> events) into the conversation_inbox read model.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	portout "pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

const (
	streamName    = "CONVERSATION_EVENTS"
	subjectPrefix = "conversation.events."
	// WildcardSubject is this stream's full subject space, the inbox
	// projector's queue-group subscription (wired in internal/fx).
	WildcardSubject = subjectPrefix + ">"
)

// SubjectForConversation is where a conversation's lifecycle events are
// published.
func SubjectForConversation(conversationID string) string {
	return subjectPrefix + conversationID
}

// EnsureStream creates (or updates, idempotently) the CONVERSATION_EVENTS
// work-queue stream. Mirrors internal/fx/nats.go's ensureNotifStream.
func EnsureStream(js natsgo.JetStreamContext) error {
	cfg := &natsgo.StreamConfig{
		Name:      streamName,
		Subjects:  []string{WildcardSubject},
		Retention: natsgo.WorkQueuePolicy,
		Storage:   natsgo.FileStorage,
	}
	if _, err := js.AddStream(cfg); err != nil {
		if _, uerr := js.UpdateStream(cfg); uerr != nil {
			return fmt.Errorf("ensure %s stream: add=%v update=%v", streamName, err, uerr)
		}
	}
	return nil
}

// envelope is the JSON wire format for every event on this stream. Type is
// the event's EventType(); Payload is that event's own JSON encoding. The
// inbox projector (inbox_projector.go, same package) is the only decoder.
type envelope struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversation_id"`
	Payload        json.RawMessage `json:"payload"`
}

// EventPublisher implements portout.EventPublisherPort over NATS JetStream.
type EventPublisher struct {
	js natsgo.JetStreamContext
}

// NewEventPublisher wires the publisher. Call EnsureStream once at
// startup before anything publishes (see internal/fx/conversation.go).
func NewEventPublisher(js natsgo.JetStreamContext) portout.EventPublisherPort {
	return &EventPublisher{js: js}
}

// conversationIDOf extracts the conversation id every event type carries.
func conversationIDOf(event domain.Event) string {
	switch e := event.(type) {
	case domain.ConversationCreated:
		return e.ConversationID.String()
	case domain.MemberAdded:
		return e.ConversationID.String()
	case domain.MemberRemoved:
		return e.ConversationID.String()
	case domain.MemberTierChanged:
		return e.ConversationID.String()
	case domain.ConversationArchived:
		return e.ConversationID.String()
	case domain.ConversationUnarchived:
		return e.ConversationID.String()
	default:
		return ""
	}
}

// Publish marshals event into the envelope and publishes it on
// conversation.events.<conversation-id>.
func (p *EventPublisher) Publish(_ context.Context, event domain.Event) error {
	convID := conversationIDOf(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", event.EventType(), err)
	}
	data, err := json.Marshal(envelope{Type: event.EventType(), ConversationID: convID, Payload: payload})
	if err != nil {
		return fmt.Errorf("marshal %s envelope: %w", event.EventType(), err)
	}
	subject := SubjectForConversation(convID)
	if _, err := p.js.Publish(subject, data); err != nil {
		return fmt.Errorf("publish %s on %s: %w", event.EventType(), subject, err)
	}
	return nil
}
