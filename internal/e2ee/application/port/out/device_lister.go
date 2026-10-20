package out

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// DeviceListerForUser reads every non-revoked device belonging to one
// account, used by ListDevices. Implemented by
// internal/e2ee/adapter/out/persistence.DeviceRepository.
type DeviceListerForUser interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*domaine2ee.Device, error)
}
