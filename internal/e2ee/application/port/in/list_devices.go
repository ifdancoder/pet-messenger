// internal/e2ee/application/port/in/list_devices.go
package in

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// ListDevicesQuery is the input to ListDevicesUseCase.
type ListDevicesQuery struct {
	Actor uuid.UUID
}

// ListDevicesUseCase returns every non-revoked device belonging to the
// actor's own account.
type ListDevicesUseCase interface {
	Handle(ctx context.Context, q ListDevicesQuery) ([]*domaine2ee.Device, error)
}
