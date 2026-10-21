package out

import (
	"context"

	"github.com/google/uuid"

	conversation "pet-messenger/internal/conversation/domain"
)

// ConversationReader is the slice of the conversation context's write-side
// repository this context needs: enough to check a call's host
// conversation exists and that the actor is a member. Consumer-owned, per
// this project's retrofit convention. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type ConversationReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*conversation.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error)
}
