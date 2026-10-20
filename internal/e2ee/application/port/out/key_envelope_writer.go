package out

import (
	"context"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// KeyEnvelopeWriter persists a batch of per-recipient key envelopes.
// Implemented by internal/e2ee/adapter/out/persistence.DeviceRepository.
type KeyEnvelopeWriter interface {
	PutKeyEnvelopes(ctx context.Context, envelopes []*domaine2ee.KeyEnvelope) error
}
