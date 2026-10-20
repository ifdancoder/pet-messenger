// internal/e2ee/application/port/in/list_conversation_devices.go
package in

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// ListConversationDevicesQuery is the input to
// ListConversationDevicesUseCase.
type ListConversationDevicesQuery struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
}

// ListConversationDevicesUseCase returns every trusted device belonging to
// any member of the conversation, for a caller who is themselves a member.
type ListConversationDevicesUseCase interface {
	Handle(ctx context.Context, q ListConversationDevicesQuery) ([]*domaine2ee.Device, error)
}
