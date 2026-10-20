package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// InboxEntry is one row of the conversation_inbox projection, already
// shaped for the ListMine response. DirectUsername/DirectDisplayName/
// DirectAvatarURL and UnreadCount are resolved by the adapter with a small
// live join against users/messages at read time: they are not
// denormalized into the projection (see the design spec).
type InboxEntry struct {
	ConversationID    uuid.UUID
	Kind              domain.Kind
	Title             string
	AvatarURL         string
	Public            bool
	CreatedAt         time.Time
	LastMessageAt     time.Time
	ArchivedAt        *time.Time
	DirectUsername    string
	DirectDisplayName string
	DirectAvatarURL   string
	UnreadCount       int
}

// InboxViewPort reads a user's conversation list from the conversation_inbox
// projection, the one query in this domain where denormalization pays for
// itself. A zero `before` starts from the newest; otherwise only rows
// strictly older than the (before, beforeID) cursor are returned.
// Implemented by
// internal/conversation/adapter/out/persistence.InboxRepository.
type InboxViewPort interface {
	ListInbox(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*InboxEntry, error)
}
