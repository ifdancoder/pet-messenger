package in

import (
	"context"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
	out "pet-messenger/internal/call/application/port/out"
)

// GetLiveForConversationQuery reads a conversation's live call and its
// current participant roster.
type GetLiveForConversationQuery struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
}

// GetLiveForConversationUseCase handles GetLiveForConversationQuery. Note:
// this "query" can write; see the Handle implementation's own doc
// comment for why.
type GetLiveForConversationUseCase interface {
	Handle(ctx context.Context, q GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error)
}
