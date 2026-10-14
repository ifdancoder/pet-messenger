package nats

import (
	"context"
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/domain/message"
	domainmsg "pet-messenger/internal/message/domain"
	usecasemsg "pet-messenger/internal/usecase/message"
)

// Publisher fans message changes out on core NATS. Delivery is best-effort: a
// failed publish returns an error for the caller to log — the message is already
// persisted and a reconnecting client recovers via GET .../messages.
type Publisher struct {
	nc *natsgo.Conn
}

// NewPublisher returns a Publisher as the usecase port it satisfies.
func NewPublisher(nc *natsgo.Conn) usecasemsg.MessagePublisher {
	return &Publisher{nc: nc}
}

func (p *Publisher) PublishCreated(_ context.Context, m *message.Message) error {
	return p.publish(MessageCreated, m)
}

func (p *Publisher) PublishUpdated(_ context.Context, m *message.Message) error {
	return p.publish(MessageUpdated, m)
}

func (p *Publisher) PublishDeleted(_ context.Context, m *message.Message) error {
	return p.publish(MessageDeleted, m)
}

func (p *Publisher) publish(t MessageEventType, m *message.Message) error {
	subject := SubjectForConversation(m.ConversationID.String())
	data, err := json.Marshal(MessageEvent{
		Type:           t,
		ConversationID: m.ConversationID.String(),
		Message:        ToPayload(toDomainMsg(m)),
	})
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", t, err)
	}
	if err := p.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

// toDomainMsg bridges the OLD internal/domain/message.Message that this
// legacy publisher still speaks to the NEW internal/message/domain.Message
// ToPayload now takes. It is a temporary type-bridging shim, not a
// restatement of the wire-payload mapping (that stays solely in ToPayload) —
// it is removed when Task 13 deletes this file along with the rest of the
// old message usecase/domain packages.
func toDomainMsg(m *message.Message) *domainmsg.Message {
	return &domainmsg.Message{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Seq:            m.Seq,
		SenderID:       m.SenderID,
		Type:           domainmsg.Type(m.Type),
		Content:        m.Content,
		SystemEvent:    domainmsg.SystemEvent(m.SystemEvent),
		SystemTargetID: m.SystemTargetID,
		ReplyToID:      m.ReplyToID,
		IdempotencyKey: m.IdempotencyKey,
		EditedAt:       m.EditedAt,
		DeletedAt:      m.DeletedAt,
		CreatedAt:      m.CreatedAt,
		ReadBy:         m.ReadBy,
		RecipientCount: m.RecipientCount,
	}
}
