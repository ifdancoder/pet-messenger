package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/domain/message"
	usecaseconv "pet-messenger/internal/usecase/conversation"
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

// NewMemberEventPublisher returns the same core-NATS Publisher as the
// conversation usecase's MemberEventPublisher port.
func NewMemberEventPublisher(nc *natsgo.Conn) usecaseconv.MemberEventPublisher {
	return &Publisher{nc: nc}
}

// PublishMemberRevoked fans a membership revocation out on member.revoked.<id>.
// Best-effort: a failed publish is the caller's to log.
func (p *Publisher) PublishMemberRevoked(_ context.Context, conversationID, userID uuid.UUID) error {
	data, err := json.Marshal(MemberRevokedEvent{
		ConversationID: conversationID.String(),
		UserID:         userID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal member revoked event: %w", err)
	}
	return p.nc.Publish(RevokeSubjectForConversation(conversationID.String()), data)
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
		Message:        toPayload(m),
	})
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", t, err)
	}
	if err := p.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
