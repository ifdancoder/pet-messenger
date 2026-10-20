// internal/e2ee/application/query/list_conversation_devices.go
package query

import (
	"context"
	"errors"

	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// ErrNotConversationMember: the actor is not a member of the conversation.
// Shared by ListConversationDevices and GetCallKeyEnvelope (via its own
// callForMember helper).
var ErrNotConversationMember = errors.New("e2ee: actor is not a conversation member")

type listConversationDevicesHandler struct {
	trustedLister out.TrustedDeviceLister
	conversations out.ConversationMemberChecker
}

// NewListConversationDevicesHandler wires the ListConversationDevices use
// case.
func NewListConversationDevicesHandler(trustedLister out.TrustedDeviceLister, conversations out.ConversationMemberChecker) in.ListConversationDevicesUseCase {
	return &listConversationDevicesHandler{trustedLister: trustedLister, conversations: conversations}
}

// Handle returns every trusted device belonging to any member of
// q.ConversationID, for a caller who is themselves a member.
func (h *listConversationDevicesHandler) Handle(ctx context.Context, q in.ListConversationDevicesQuery) ([]*domaine2ee.Device, error) {
	if _, err := h.conversations.GetMember(ctx, q.ConversationID, q.Actor); err != nil {
		return nil, ErrNotConversationMember
	}
	return h.trustedLister.ListTrustedForConversation(ctx, q.ConversationID)
}
