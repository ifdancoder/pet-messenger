// internal/message/application/port/out/notification_producer.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// NotificationProducer enqueues a "something happened that a user may need
// to be notified about" intent onto the notifications work queue, after the
// write commits. Implemented by
// internal/notification/adapter/out/messaging.Producer.
// Delivery is best-effort - same contract as MessagePublisher - so a command
// handler logs an error and still returns success.
type NotificationProducer interface {
	// MessageSent is called for every newly created message. The producer
	// decides (by conversation kind) whether an intent is actually emitted.
	MessageSent(ctx context.Context, m *domainmsg.Message, conv *domainconv.Conversation) error
	// MembershipEvent is called for a kick (SystemMemberRemoved) or a tier
	// change (SystemMemberTierChanged). newTier is the resulting tier for a
	// tier change and "" otherwise.
	MembershipEvent(ctx context.Context, event domainmsg.SystemEvent, conversationID, targetUserID uuid.UUID, newTier string) error
}
