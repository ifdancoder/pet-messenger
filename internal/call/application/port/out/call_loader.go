package out

import (
	"context"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// CallLoader is the read half of call persistence.
type CallLoader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Call, error)
	// GetLiveForConversation returns the conversation's single active
	// call, or domain.ErrNotFound when there is none.
	GetLiveForConversation(ctx context.Context, conversationID uuid.UUID) (*domain.Call, error)
	// ListLive returns every currently-active call, oldest first.
	ListLive(ctx context.Context) ([]*domain.Call, error)
}
