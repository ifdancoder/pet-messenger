// internal/notification/adapter/out/messaging/producer.go

// Package messaging is the notification context's messaging adapters: the
// publish side (this file) and, in adapter/in/messaging, the consume side.
// NotifIntent is declared here because the publish side owns the wire
// format; the consume side imports it from this package.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// NotifIntent is the wire payload published to
// notif.msg.<id> / notif.member.<id> / notif.call.<id>. The worker resolves
// recipients and dedups on "<DedupPrefix>:<ConversationID>:<MessageID|RecipientID>".
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

// Producer publishes NotifIntent messages onto the NOTIF work queue and
// per-user WS payloads onto core NATS. Structurally satisfies message's own
// NotificationProducer port, usecase/call's own CallNotifier port, and this
// context's own WSNotifier port.
type Producer struct {
	js natsgo.JetStreamContext
	nc *natsgo.Conn
}

// NewProducer wires the producer against a JetStream context and the plain
// NATS connection it is derived from (provided by NATSModule). The NOTIF
// stream is ensured on boot by ensureNotifStream.
func NewProducer(js natsgo.JetStreamContext, nc *natsgo.Conn) *Producer {
	return &Producer{js: js, nc: nc}
}

// firstRunes returns at most the first n runes of s (rune-safe truncation
// for the message preview).
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// MessageSent publishes one notif.msg.<message-id> intent for a direct or
// group chat message. Channel messages produce no per-message notification
// here (the channel-follow fan-out is a separate concern), so it is a no-op
// for them.
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
// (member_removed) or a tier change (member_tier_changed). Any other event
// is a no-op.
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

// CallStarted publishes one notif.call.<callID> intent when a call starts,
// so the notification worker rings every other conversation member (online
// via WS, offline via device push). Best-effort: the caller logs and
// swallows errors.
func (p *Producer) CallStarted(_ context.Context, callID, conversationID, initiatorID uuid.UUID) error {
	sid := initiatorID.String()
	data, err := json.Marshal(NotifIntent{
		Kind:           "call_started",
		ConversationID: conversationID.String(),
		SenderID:       &sid,
		CallID:         callID.String(),
		DedupPrefix:    "call",
	})
	if err != nil {
		return fmt.Errorf("marshal call notif intent: %w", err)
	}
	if _, err := p.js.Publish("notif.call."+callID.String(), data); err != nil {
		return fmt.Errorf("publish call notif intent: %w", err)
	}
	return nil
}

// Notify publishes a per-user notification payload on core NATS
// (notif.ws.<uid>); a per-pod subscription feeds it to Hub.SendToUser.
func (p *Producer) Notify(userID uuid.UUID, payload []byte) error {
	return p.nc.Publish("notif.ws."+userID.String(), payload)
}
