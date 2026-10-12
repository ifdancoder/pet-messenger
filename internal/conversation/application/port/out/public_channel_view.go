package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// PublicChannelViewPort reads discoverable channels directly off the
// write-side tables — this query has no dedicated projection (see the
// design spec's "Read model scope"). Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type PublicChannelViewPort interface {
	ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domain.Conversation, error)
}
