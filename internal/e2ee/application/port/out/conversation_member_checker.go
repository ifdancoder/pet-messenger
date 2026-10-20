package out

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
)

// ConversationMemberChecker confirms the actor is a member of a conversation
// before trusting an e2ee device claim scoped to it. Consumer-owned by this
// context (the same shape as today's local interface in the `http`
// package). Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type ConversationMemberChecker interface {
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domainconv.Member, error)
}
