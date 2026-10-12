package out

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// UpdateConversationPort is the write side of the conversation aggregate.
// Implemented by internal/conversation/adapter/out/persistence.ConversationRepository.
type UpdateConversationPort interface {
	// Create persists the conversation and its founding members atomically.
	Create(ctx context.Context, c *domain.Conversation, members []*domain.Member) error
	// AddMember inserts one membership row; idempotent (nil if already a member).
	AddMember(ctx context.Context, m *domain.Member) error
	// RemoveMember deletes a membership row, or domain.ErrNotMember if none.
	RemoveMember(ctx context.Context, conversationID, userID uuid.UUID) error
	SetMemberTier(ctx context.Context, conversationID, userID uuid.UUID, tier domain.AdminTier) error
	SetArchived(ctx context.Context, conversationID uuid.UUID, archived bool) error
	// RemoveMemberWithSuccession deletes userID's membership and, if that row
	// held is_creator, promotes the highest-ranked remaining member in the
	// same transaction. promoted is nil when the leaver was not the creator
	// or no members remain.
	RemoveMemberWithSuccession(ctx context.Context, conversationID, userID uuid.UUID) (promoted *domain.Member, err error)
	ClaimCreator(ctx context.Context, conversationID, userID uuid.UUID) (*domain.Member, error)
}
