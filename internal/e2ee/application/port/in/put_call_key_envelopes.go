// internal/e2ee/application/port/in/put_call_key_envelopes.go
package in

import (
	"context"

	"github.com/google/uuid"
)

// KeyEnvelopeInput is one client-submitted key envelope within a
// PutCallKeyEnvelopesCommand.
type KeyEnvelopeInput struct {
	RecipientDeviceID uuid.UUID
	Ciphertext        []byte
}

// PutCallKeyEnvelopesCommand is the input to PutCallKeyEnvelopesUseCase.
type PutCallKeyEnvelopesCommand struct {
	Actor         uuid.UUID
	CallID        uuid.UUID
	OwnerDeviceID uuid.UUID
	Envelopes     []KeyEnvelopeInput
}

// PutCallKeyEnvelopesUseCase publishes a call's encrypted session key,
// wrapped once per trusted device in the call's conversation. The actor
// must be the call's initiator, and the submitted envelope set must cover
// every trusted device exactly once.
type PutCallKeyEnvelopesUseCase interface {
	Handle(ctx context.Context, cmd PutCallKeyEnvelopesCommand) error
}
