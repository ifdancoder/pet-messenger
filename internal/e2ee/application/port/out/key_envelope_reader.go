package out

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// KeyEnvelopeReader reads the most recent key envelope for one recipient
// device in one scope (e.g. a specific call). Implemented by
// internal/e2ee/adapter/out/persistence.DeviceRepository.
type KeyEnvelopeReader interface {
	GetKeyEnvelope(ctx context.Context, recipientID uuid.UUID, scope string) (*domaine2ee.KeyEnvelope, error)
}
