// internal/message/application/port/in/list_messages.go
package in

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// ListMessagesQuery is the input to ListMessagesUseCase. At most one of
// AfterSeq / BeforeSeq may be set: AfterSeq walks forward (ascending, the
// reconnect catch-up read), BeforeSeq walks backward (descending, history
// scroll). Both nil means "newest page" (descending from the end).
type ListMessagesQuery struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	AfterSeq       *int64
	BeforeSeq      *int64
	Limit          int
}

// ListMessagesUseCase returns a page of a conversation's history for a
// member, annotated with per-message read status.
type ListMessagesUseCase interface {
	Handle(ctx context.Context, q ListMessagesQuery) ([]*domainmsg.Message, error)
}
