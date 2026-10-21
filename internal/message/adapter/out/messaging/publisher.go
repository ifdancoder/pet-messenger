// Package messaging is the message context's secondary ("driven")
// real-time adapter: it implements out.MessagePublisher over the shared
// core-NATS wire schema in internal/infrastructure/nats, which is also
// consumed directly by conversation's own InboxProjector and the WS Hub
// subscriber (internal/fx/nats.go): that schema stays shared rather than
// moving here, since those other two consumers need it too.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	natsinfra "pet-messenger/internal/platform/infrastructure/nats"
	"pet-messenger/internal/message/application/port/out"
	domainmsg "pet-messenger/internal/message/domain"
)

// Publisher fans message changes out on core NATS. Delivery is best-effort:
// a failed publish returns an error for the caller to log; the message is
// already persisted and a reconnecting client recovers via
// GET .../messages.
type Publisher struct {
	nc *natsgo.Conn
}

// NewPublisher returns a Publisher as the port it satisfies.
func NewPublisher(nc *natsgo.Conn) out.MessagePublisher {
	return &Publisher{nc: nc}
}

// PublishCreated publishes a message.created event.
func (p *Publisher) PublishCreated(_ context.Context, m *domainmsg.Message) error {
	return p.publish(natsinfra.MessageCreated, m)
}

// PublishUpdated publishes a message.updated event.
func (p *Publisher) PublishUpdated(_ context.Context, m *domainmsg.Message) error {
	return p.publish(natsinfra.MessageUpdated, m)
}

// PublishDeleted publishes a message.deleted event.
func (p *Publisher) PublishDeleted(_ context.Context, m *domainmsg.Message) error {
	return p.publish(natsinfra.MessageDeleted, m)
}

func (p *Publisher) publish(t natsinfra.MessageEventType, m *domainmsg.Message) error {
	subject := natsinfra.SubjectForConversation(m.ConversationID.String())
	data, err := json.Marshal(natsinfra.MessageEvent{
		Type:           t,
		ConversationID: m.ConversationID.String(),
		Message:        natsinfra.ToPayload(m),
	})
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", t, err)
	}
	if err := p.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
