package message

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
)

// MessagePublisher broadcasts a message change to connected clients across pods,
// after the write commits. Implemented by internal/infrastructure/nats.Publisher.
// Delivery is best-effort — the message is already persisted and a reconnecting
// client recovers via ListMessages — so the Service logs a publish error and
// still returns success.
//
// Consumers must order by the message's Seq; NATS/WS arrival order is not
// guaranteed (concurrent sends can interleave between the DB write and the
// publish).
type MessagePublisher interface {
	PublishCreated(ctx context.Context, m *domainmsg.Message) error
	PublishUpdated(ctx context.Context, m *domainmsg.Message) error
	PublishDeleted(ctx context.Context, m *domainmsg.Message) error
}

// NotificationProducer enqueues a "something happened that a user may need to be
// notified about" intent onto the notifications work queue, after the write
// commits. Implemented by internal/infrastructure/notification.Producer.
// Delivery is best-effort — same contract as MessagePublisher — so the Service
// logs an error and still returns success.
type NotificationProducer interface {
	// MessageSent is called for every newly created message. The producer
	// decides (by conversation kind) whether an intent is actually emitted.
	MessageSent(ctx context.Context, m *domainmsg.Message, conv *domainconv.Conversation) error
	// MembershipEvent is called for a kick (SystemMemberRemoved) or a tier
	// change (SystemMemberTierChanged). newTier is the resulting tier for a tier
	// change and "" otherwise.
	MembershipEvent(ctx context.Context, event domainmsg.SystemEvent, conversationID, targetUserID uuid.UUID, newTier string) error
}
