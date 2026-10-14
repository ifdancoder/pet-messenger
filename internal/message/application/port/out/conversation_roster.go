// internal/message/application/port/out/conversation_roster.go
package out

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
)

// ConversationRosterPort reads a conversation's roster, used only by
// ListMessages' read-status annotation (who else is in the conversation, and
// what have they read). Consumer-owned by this context. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type ConversationRosterPort interface {
	ListMembers(ctx context.Context, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.Member, error)
}
