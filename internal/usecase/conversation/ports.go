package conversation

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
)

// MemberEventPublisher fans "this user's membership was revoked" out across api
// pods so every pod can drop the departed member's live WebSocket subscription.
// Implemented by internal/infrastructure/nats.Publisher.
type MemberEventPublisher interface {
	PublishMemberRevoked(ctx context.Context, conversationID, userID uuid.UUID) error
}

// SystemEventPoster records a membership / lifecycle change as a system message
// in the conversation's timeline. It no-ops for direct conversations and (for
// member_* events) for channel_broadcast. Implemented by
// *internal/usecase/message.Service.
type SystemEventPoster interface {
	PostSystemEvent(ctx context.Context, conv *domainconv.Conversation, actorID *uuid.UUID, event domainmsg.SystemEvent, targetID uuid.UUID, detail string) error
}
