// internal/message/application/port/out/read_cursor.go
package out

import (
	"context"

	"github.com/google/uuid"
)

// ReadCursorPort advances a member's last-read cursor, used only by
// MarkRead. Consumer-owned by this context. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository —
// SetLastReadMessage is a plain public method there, not part of
// conversation's own published port/out, since only this context needs it.
type ReadCursorPort interface {
	SetLastReadMessage(ctx context.Context, conversationID, userID, messageID uuid.UUID) error
}
