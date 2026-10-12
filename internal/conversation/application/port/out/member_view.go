package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// MemberViewPort reads a conversation's roster directly off the write-side
// tables — no dedicated projection. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type MemberViewPort interface {
	ListMembers(ctx context.Context, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domain.MemberProfile, error)
}
