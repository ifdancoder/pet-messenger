package out

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// DeviceWriter is the write side of the device trust lifecycle. Implemented
// by internal/e2ee/adapter/out/persistence.DeviceRepository.
type DeviceWriter interface {
	// Register persists d, deciding under a row lock whether it becomes the
	// account's first (and therefore auto-trusted) device or a pending one
	// awaiting approval. On success d.Status is set to reflect that decision.
	Register(ctx context.Context, d *domaine2ee.Device) error
	// Approve trusts targetID on approverID's vouching. Both devices must
	// belong to the same user; the caller has already verified the approval
	// signature before calling this.
	Approve(ctx context.Context, approverID, targetID uuid.UUID) error
}
