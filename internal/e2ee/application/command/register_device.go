// Package command holds the CQRS write side of the e2ee context: one
// handler per use case, each implementing the matching port/in use case
// and depending only on the port/out interfaces it actually needs.
package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// Sentinel errors.
var (
	// ErrDeviceIDConflict: the submitted device id is already registered
	// under a different owner or different keys.
	ErrDeviceIDConflict = errors.New("e2ee: device id is already registered")
	// ErrDeviceAlreadyRegistered: the device id was free at read time, but a
	// concurrent registration won the race.
	ErrDeviceAlreadyRegistered = errors.New("e2ee: device is already registered")
)

type registerDeviceHandler struct {
	loader out.DeviceLoader
	writer out.DeviceWriter
}

// NewRegisterDeviceHandler wires the RegisterDevice use case.
func NewRegisterDeviceHandler(loader out.DeviceLoader, writer out.DeviceWriter) in.RegisterDeviceUseCase {
	return &registerDeviceHandler{loader: loader, writer: writer}
}

// Handle registers cmd.DeviceID for cmd.Actor. A retry with the exact same
// id and keys returns the existing device unchanged (created=false,
// idempotent); the same id with a different owner or different keys is
// ErrDeviceIDConflict.
func (h *registerDeviceHandler) Handle(ctx context.Context, cmd in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
	d, err := domaine2ee.NewDevice(cmd.DeviceID, cmd.Actor, cmd.PublicKey, cmd.SigningPublicKey, cmd.Label)
	if err != nil {
		return nil, false, err // domain.ErrInvalidKey
	}

	existing, err := h.loader.Get(ctx, cmd.DeviceID)
	if err == nil {
		same := existing.UserID == cmd.Actor && equalBytes(existing.PublicKey, cmd.PublicKey) && equalBytes(existing.SigningPublicKey, cmd.SigningPublicKey)
		if !same {
			return nil, false, ErrDeviceIDConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, domaine2ee.ErrNotFound) {
		return nil, false, fmt.Errorf("load existing device: %w", err)
	}

	d.CreatedAt = time.Now().UTC()
	if err := h.writer.Register(ctx, d); err != nil {
		return nil, false, ErrDeviceAlreadyRegistered
	}
	return d, true, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
