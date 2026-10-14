// internal/notification/application/port/out/member_lister.go
package out

import (
	"context"

	"github.com/google/uuid"
)

// MemberLister is the slice of the conversation context's write-side
// repository this context needs: just enough to fan a notification intent
// out to every current member. Implemented by
// internal/conversation/adapter/out/persistence.ConversationRepository.
type MemberLister interface {
	ListMemberIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error)
}
