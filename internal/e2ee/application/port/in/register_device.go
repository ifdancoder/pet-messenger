// internal/e2ee/application/port/in/register_device.go

// Package in holds the e2ee application layer's inbound ("driving")
// ports: one use-case interface per command/query, each with its own
// parameter struct.
package in

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// RegisterDeviceCommand is the input to RegisterDeviceUseCase.
type RegisterDeviceCommand struct {
	Actor            uuid.UUID
	DeviceID         uuid.UUID
	PublicKey        []byte
	SigningPublicKey []byte
	Label            string
}

// RegisterDeviceUseCase registers a new e2ee device for the actor, or
// returns the actor's own already-registered device unchanged if the same
// id/keys were submitted again. created is true for a genuinely new
// registration (the controller responds 201) and false for an idempotent
// replay of an identical prior registration (the controller responds 200)
// -- the original handler makes exactly this status-code distinction, so
// the use case must report which case happened, not just the device.
type RegisterDeviceUseCase interface {
	Handle(ctx context.Context, cmd RegisterDeviceCommand) (device *domaine2ee.Device, created bool, err error)
}
