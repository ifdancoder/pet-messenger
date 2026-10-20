// internal/e2ee/application/query/list_devices_test.go
package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/e2ee/application/port/in"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

type fakeDeviceListerForUser struct {
	byUser map[uuid.UUID][]*domaine2ee.Device
	err    error
}

func (f *fakeDeviceListerForUser) ListForUser(_ context.Context, userID uuid.UUID) ([]*domaine2ee.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byUser[userID], nil
}

func TestListDevices_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	want := []*domaine2ee.Device{{ID: uuid.Must(uuid.NewV7()), UserID: actor}}
	h := NewListDevicesHandler(&fakeDeviceListerForUser{byUser: map[uuid.UUID][]*domaine2ee.Device{actor: want}})

	got, err := h.Handle(context.Background(), in.ListDevicesQuery{Actor: actor})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListDevices_PropagatesError(t *testing.T) {
	h := NewListDevicesHandler(&fakeDeviceListerForUser{err: errors.New("db down")})
	if _, err := h.Handle(context.Background(), in.ListDevicesQuery{Actor: uuid.Must(uuid.NewV7())}); err == nil {
		t.Fatal("want a non-nil error")
	}
}
