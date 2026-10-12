package in

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type ListMembersQuery struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	AdminsOnly     bool
	Before         time.Time
	BeforeUserID   uuid.UUID
	Limit          int
}

// ListMembersUseCase returns a page of a conversation's roster. Any current
// member may read it.
type ListMembersUseCase interface {
	Handle(ctx context.Context, q ListMembersQuery) ([]*domain.MemberProfile, error)
}
