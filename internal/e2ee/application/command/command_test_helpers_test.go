// internal/e2ee/application/command/command_test_helpers_test.go
package command

import (
	"context"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

type fakeDeviceLoader struct {
	byID map[uuid.UUID]*domaine2ee.Device
}

func (f *fakeDeviceLoader) Get(_ context.Context, id uuid.UUID) (*domaine2ee.Device, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return nil, domaine2ee.ErrNotFound
}

type fakeDeviceWriter struct {
	registered  []*domaine2ee.Device
	registerErr error
	approved    []approval
	approveErr  error
}

type approval struct {
	Approver, Target uuid.UUID
}

func (f *fakeDeviceWriter) Register(_ context.Context, d *domaine2ee.Device) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, d)
	return nil
}

func (f *fakeDeviceWriter) Approve(_ context.Context, approver, target uuid.UUID) error {
	if f.approveErr != nil {
		return f.approveErr
	}
	f.approved = append(f.approved, approval{Approver: approver, Target: target})
	return nil
}
