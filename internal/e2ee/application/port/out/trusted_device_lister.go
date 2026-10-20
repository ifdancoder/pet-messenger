package out

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// TrustedDeviceLister reads every trusted device belonging to any member of
// one conversation, used both to authorize a call key-envelope submission
// (every trusted device must receive exactly one envelope) and to list a
// conversation's encryption-relevant devices. Implemented by
// internal/e2ee/adapter/out/persistence.DeviceRepository.
type TrustedDeviceLister interface {
	ListTrustedForConversation(ctx context.Context, conversationID uuid.UUID) ([]*domaine2ee.Device, error)
}
