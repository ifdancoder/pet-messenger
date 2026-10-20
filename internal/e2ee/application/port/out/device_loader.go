// Package out holds the e2ee application layer's outbound ("driven")
// ports: what the use cases need from the outside world. Implemented by
// internal/e2ee/adapter/out/persistence (the device/key-envelope ports) and,
// for the two cross-context ports, by internal/conversation/adapter/out/
// persistence and internal/infrastructure/postgres (call's persistence,
// unchanged until the `call` slice's own retrofit).
package out

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// DeviceLoader reads a single device by id, the pre-write-check read every
// command needs before deciding whether an action is allowed. Implemented
// by internal/e2ee/adapter/out/persistence.DeviceRepository.
type DeviceLoader interface {
	Get(ctx context.Context, id uuid.UUID) (*domaine2ee.Device, error)
}
