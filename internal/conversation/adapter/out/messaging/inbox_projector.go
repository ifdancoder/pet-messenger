// internal/conversation/adapter/out/messaging/inbox_projector.go
package messaging

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/conversation/adapter/out/persistence"
	"pet-messenger/internal/conversation/domain"
	natsmsg "pet-messenger/internal/platform/infrastructure/nats"
)

// inboxWriter is the slice of persistence.InboxRepository this projector
// needs. Defined here, not in application/port/out, because it is
// consumed only by this adapter talking to a sibling adapter, never by the
// application layer.
type inboxWriter interface {
	UpsertMember(ctx context.Context, row persistence.InboxRow) error
	RemoveMember(ctx context.Context, conversationID, userID uuid.UUID, eventSeq int64) error
	UpdateArchived(ctx context.Context, conversationID uuid.UUID, archivedAt *time.Time, eventSeq int64) error
	UpdateLastMessageAt(ctx context.Context, conversationID uuid.UUID, lastMessageAt time.Time) error
}

// InboxProjector consumes this context's own lifecycle events
// (conversation.events.>, via HandleLifecycleEvent) and the message
// domain's existing msg.> events (via HandleMessageEvent), keeping the
// conversation_inbox read model current. It is the one subscriber in this
// system where losing an event causes permanent read-model drift (unlike
// the WS fan-out, where a disconnected client re-reads Postgres directly):
// see the design spec.
type InboxProjector struct {
	inbox inboxWriter
}

// NewInboxProjector wires the projector.
func NewInboxProjector(inbox inboxWriter) *InboxProjector {
	return &InboxProjector{inbox: inbox}
}

// HandleLifecycleEvent is the JetStream durable-consumer callback for
// conversation.events.>. It never returns an error: unparseable messages
// are logged and Term'd (never retried: retrying cannot fix a decode
// error); a downstream write failure is Nak'd for redelivery.
func (p *InboxProjector) HandleLifecycleEvent(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var env envelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		log.Printf("conversation: inbox projector: bad envelope on %s: %v", m.Subject, err)
		_ = m.Term()
		return
	}
	seq := streamSeq(m)

	var applyErr error
	switch env.Type {
	case "member.added":
		var ev domain.MemberAdded
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			log.Printf("conversation: inbox projector: bad member.added payload: %v", err)
			_ = m.Term()
			return
		}
		applyErr = p.inbox.UpsertMember(ctx, persistence.InboxRow{
			UserID: ev.UserID, ConversationID: ev.ConversationID, Kind: ev.Kind,
			Title: ev.Title, AvatarURL: ev.AvatarURL, Public: ev.Public,
			CreatedAt: ev.ConversationCreatedAt, DirectPeerUserID: ev.DirectPeerUserID, EventSeq: seq,
		})
	case "member.removed":
		var ev domain.MemberRemoved
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			log.Printf("conversation: inbox projector: bad member.removed payload: %v", err)
			_ = m.Term()
			return
		}
		applyErr = p.inbox.RemoveMember(ctx, ev.ConversationID, ev.UserID, seq)
	case "conversation.archived":
		var ev domain.ConversationArchived
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			log.Printf("conversation: inbox projector: bad conversation.archived payload: %v", err)
			_ = m.Term()
			return
		}
		now := time.Now().UTC()
		applyErr = p.inbox.UpdateArchived(ctx, ev.ConversationID, &now, seq)
	case "conversation.unarchived":
		var ev domain.ConversationUnarchived
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			log.Printf("conversation: inbox projector: bad conversation.unarchived payload: %v", err)
			_ = m.Term()
			return
		}
		applyErr = p.inbox.UpdateArchived(ctx, ev.ConversationID, nil, seq)
	case "conversation.created", "member.tier_changed":
		// Not consumed by the inbox projection (see the design spec).
	default:
		log.Printf("conversation: inbox projector: unknown event type %q on %s", env.Type, m.Subject)
	}
	if applyErr != nil {
		log.Printf("conversation: inbox projector: apply %s: %v", env.Type, applyErr)
		_ = m.Nak()
		return
	}
	_ = m.Ack()
}

// HandleMessageEvent is the per-pod core-NATS callback for msg.> (message
// domain events). It only reacts to message.created, advancing
// last_message_at the same way the pre-retrofit MessageRepository.Create
// did (every message type, including system messages, bumps it).
func (p *InboxProjector) HandleMessageEvent(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var ev natsmsg.MessageEvent
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		log.Printf("conversation: inbox projector: bad message event on %s: %v", m.Subject, err)
		return
	}
	if ev.Type != natsmsg.MessageCreated {
		return
	}
	convID, err := uuid.Parse(ev.ConversationID)
	if err != nil {
		log.Printf("conversation: inbox projector: bad conversation id %q", ev.ConversationID)
		return
	}
	if err := p.inbox.UpdateLastMessageAt(ctx, convID, ev.Message.CreatedAt); err != nil {
		log.Printf("conversation: inbox projector: update last_message_at for %s: %v", convID, err)
	}
}

// streamSeq reads the JetStream stream sequence number off a delivered
// message. Always present for a JetStream subscription; HandleMessageEvent
// never calls this (msg.> is plain core NATS, with no such sequence).
func streamSeq(m *natsgo.Msg) int64 {
	meta, err := m.Metadata()
	if err != nil {
		return 0
	}
	return int64(meta.Sequence.Stream)
}
