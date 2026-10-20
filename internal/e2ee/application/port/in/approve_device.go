// internal/e2ee/application/port/in/approve_device.go
package in

import (
	"context"

	"github.com/google/uuid"
)

// ApproveDeviceCommand is the input to ApproveDeviceUseCase.
type ApproveDeviceCommand struct {
	Actor      uuid.UUID
	ApproverID uuid.UUID
	TargetID   uuid.UUID
	Signature  []byte
}

// ApproveDeviceUseCase trusts the actor's own pending device (TargetID) on
// the vouching of one of the actor's already-trusted devices (ApproverID),
// verified by the approver's own signature over the target's fingerprint.
type ApproveDeviceUseCase interface {
	Handle(ctx context.Context, cmd ApproveDeviceCommand) error
}
