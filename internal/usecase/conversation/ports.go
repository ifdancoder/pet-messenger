package conversation

import (
	"context"

	"github.com/google/uuid"
)

// MemberEventPublisher fans "this user's membership was revoked" out across api
// pods so every pod can drop the departed member's live WebSocket subscription.
// Implemented by internal/infrastructure/nats.Publisher.
type MemberEventPublisher interface {
	PublishMemberRevoked(ctx context.Context, conversationID, userID uuid.UUID) error
}
