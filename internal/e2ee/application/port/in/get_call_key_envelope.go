// internal/e2ee/application/port/in/get_call_key_envelope.go
package in

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// GetCallKeyEnvelopeQuery is the input to GetCallKeyEnvelopeUseCase.
type GetCallKeyEnvelopeQuery struct {
	Actor            uuid.UUID
	CallID           uuid.UUID
	RecipientDeviceID uuid.UUID
}

// GetCallKeyEnvelopeUseCase returns the call's encrypted session key,
// wrapped for the actor's own recipient device, once the owning device has
// published it.
type GetCallKeyEnvelopeUseCase interface {
	Handle(ctx context.Context, q GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error)
}
