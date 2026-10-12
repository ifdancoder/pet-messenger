// Package out holds the conversation application layer's outbound
// ("driven") ports: what the use cases need from the outside world.
// Implemented by internal/conversation/adapter/out/persistence and
// internal/conversation/adapter/out/messaging.
package out

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// LoadConversationPort reads a conversation or one of its memberships for
// the write side: every command re-derives current state before deciding
// whether an action is allowed. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type LoadConversationPort interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domain.Member, error)
	// GetDirect returns the direct conversation between userA and userB
	// regardless of argument order, or domain.ErrNotFound if there is none.
	GetDirect(ctx context.Context, userA, userB uuid.UUID) (*domain.Conversation, error)
}
