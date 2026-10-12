package message

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

// ConversationReader is the slice of the conversation context's write-side
// repository this service needs: enough to check membership/role before a
// message write, and to advance a member's read cursor. Defined here
// (consumer-driven), not imported from the conversation context, per the
// project's retrofit design (see
// docs/superpowers/specs/2026-10-12-conversation-clean-hexagonal-cqrs-design.md).
// Implemented by internal/conversation/adapter/out/persistence.ConversationRepository.
type ConversationReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domainconv.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domainconv.Member, error)
	SetLastReadMessage(ctx context.Context, conversationID, userID, messageID uuid.UUID) error
	ListMembers(ctx context.Context, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.MemberProfile, error)
}

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
