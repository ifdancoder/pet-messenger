// internal/e2ee/application/query/list_devices.go

// Package query holds the CQRS read side of the e2ee context.
package query

import (
	"context"

	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

type listDevicesHandler struct {
	lister out.DeviceListerForUser
}

// NewListDevicesHandler wires the ListDevices use case.
func NewListDevicesHandler(lister out.DeviceListerForUser) in.ListDevicesUseCase {
	return &listDevicesHandler{lister: lister}
}

// Handle returns every non-revoked device belonging to q.Actor's account.
func (h *listDevicesHandler) Handle(ctx context.Context, q in.ListDevicesQuery) ([]*domaine2ee.Device, error) {
	return h.lister.ListForUser(ctx, q.Actor)
}
