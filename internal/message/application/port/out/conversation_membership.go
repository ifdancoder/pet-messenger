// internal/message/application/port/out/conversation_membership.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
)

// ConversationMembershipPort reads a conversation or one of its memberships
// for the write side: every command re-derives current state before
// deciding whether an action is allowed. Consumer-owned by this context —
// defined here rather than imported from conversation's own ports — since
// this context only ever needs these two methods. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type ConversationMembershipPort interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domainconv.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domainconv.Member, error)
}
