// Package notification is the infrastructure side of the notifications
// subsystem: it turns a domain event into a NotifIntent JSON message published
// onto the NOTIF JetStream work queue, where the notification worker picks it
// up, fans it out to conversation members, and delivers via WebSocket / device
// push.
package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	domainmsg "pet-messenger/internal/message/domain"
	messageout "pet-messenger/internal/message/application/port/out"

	domainconv "pet-messenger/internal/conversation/domain"
)

// NotifIntent is the wire payload published to notif.msg.<id> / notif.member.<id>.
// The worker resolves recipients and dedups on
// "<DedupPrefix>:<ConversationID>:<MessageID|RecipientID>".
type NotifIntent struct {
	Kind           string  `json:"kind"`
	ConversationID string  `json:"conversation_id"`
	MessageID      *string `json:"message_id,omitempty"`
	MessageSeq     *int64  `json:"message_seq,omitempty"`
	SenderID       *string `json:"sender_id,omitempty"`
	Preview        string  `json:"preview,omitempty"`
	RecipientID    *string `json:"recipient_id,omitempty"`
	NewTier        string  `json:"new_tier,omitempty"`
	CallID         string  `json:"call_id,omitempty"`
	DedupPrefix    string  `json:"dedup_prefix"`
}

// Producer publishes NotifIntent messages onto the NOTIF work queue.
type Producer struct{ js natsgo.JetStreamContext }

// NewProducer wires the producer against a JetStream context (provided by
// NATSModule). The NOTIF stream is ensured on boot by ensureNotifStream.
func NewProducer(js natsgo.JetStreamContext) messageout.NotificationProducer {
	return &Producer{js: js}
}

// firstRunes returns at most the first n runes of s (rune-safe truncation for
// the message preview).
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// MessageSent publishes one notif.msg.<message-id> intent for a direct or group
// chat message. Channel messages produce no per-message notification here (the
// channel-follow fan-out is a separate concern), so it is a no-op for them.
func (p *Producer) MessageSent(_ context.Context, m *domainmsg.Message, conv *domainconv.Conversation) error {
	if conv.Kind != domainconv.KindDirect && conv.Kind != domainconv.KindGroupChat {
		return nil
	}
	id := m.ID.String()
	intent := NotifIntent{
		Kind: "message", ConversationID: conv.ID.String(),
		MessageID: &id, MessageSeq: &m.Seq, Preview: firstRunes(m.Content, 140),
		DedupPrefix: "msg",
	}
	if m.SenderID != nil {
		s := m.SenderID.String()
		intent.SenderID = &s
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal message notif intent: %w", err)
	}
	if _, err := p.js.Publish("notif.msg."+id, data); err != nil {
		return fmt.Errorf("publish message notif intent: %w", err)
	}
	return nil
}

// MembershipEvent publishes one notif.member.<random> intent for a kick
// (member_removed) or a tier change (member_tier_changed). Any other event is a
// no-op.
func (p *Producer) MembershipEvent(_ context.Context, event domainmsg.SystemEvent, conversationID, targetUserID uuid.UUID, newTier string) error {
	var kind, prefix string
	switch event {
	case domainmsg.SystemMemberRemoved:
		kind, prefix = "member_removed", "kick"
	case domainmsg.SystemMemberTierChanged:
		kind, prefix = "member_tier_changed", "tier"
	default:
		return nil
	}
	rid := targetUserID.String()
	data, err := json.Marshal(NotifIntent{
		Kind: kind, ConversationID: conversationID.String(),
		RecipientID: &rid, NewTier: newTier, DedupPrefix: prefix,
	})
	if err != nil {
		return fmt.Errorf("marshal member notif intent: %w", err)
	}
	if _, err := p.js.Publish("notif.member."+uuid.NewString(), data); err != nil {
		return fmt.Errorf("publish member notif intent: %w", err)
	}
	return nil
}
