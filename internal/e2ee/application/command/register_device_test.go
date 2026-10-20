// internal/e2ee/application/command/register_device_test.go
package command

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/e2ee/application/port/in"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

func testKeys(t *testing.T) (publicKey, signingPublicKey []byte) {
	t.Helper()
	publicKey = make([]byte, 32)
	publicKey[0] = 9
	signingPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	return publicKey, signingPublicKey
}

func TestRegisterDevice_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey, signingPublicKey := testKeys(t)
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{}}
	writer := &fakeDeviceWriter{}
	h := NewRegisterDeviceHandler(loader, writer)

	d, created, err := h.Handle(context.Background(), in.RegisterDeviceCommand{Actor: actor, DeviceID: deviceID, PublicKey: publicKey, SigningPublicKey: signingPublicKey, Label: "phone"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !created {
		t.Error("created = false, want true for a brand-new registration")
	}
	if d.ID != deviceID || d.UserID != actor || d.Label != "phone" {
		t.Errorf("device = %+v", d)
	}
	if len(writer.registered) != 1 {
		t.Fatalf("Register called %d times, want 1", len(writer.registered))
	}
}

func TestRegisterDevice_IdempotentReplay(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey, signingPublicKey := testKeys(t)
	existing := &domaine2ee.Device{ID: deviceID, UserID: actor, PublicKey: publicKey, SigningPublicKey: signingPublicKey, Status: domaine2ee.StatusTrusted}
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{deviceID: existing}}
	writer := &fakeDeviceWriter{}
	h := NewRegisterDeviceHandler(loader, writer)

	d, created, err := h.Handle(context.Background(), in.RegisterDeviceCommand{Actor: actor, DeviceID: deviceID, PublicKey: publicKey, SigningPublicKey: signingPublicKey})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if created {
		t.Error("created = true, want false for an identical replay")
	}
	if d != existing {
		t.Errorf("got %+v, want the existing device unchanged", d)
	}
	if len(writer.registered) != 0 {
		t.Fatalf("Register called %d times, want 0", len(writer.registered))
	}
}

func TestRegisterDevice_IDConflict(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey, signingPublicKey := testKeys(t)
	existing := &domaine2ee.Device{ID: deviceID, UserID: other, PublicKey: publicKey, SigningPublicKey: signingPublicKey}
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{deviceID: existing}}
	h := NewRegisterDeviceHandler(loader, &fakeDeviceWriter{})

	_, _, err := h.Handle(context.Background(), in.RegisterDeviceCommand{Actor: actor, DeviceID: deviceID, PublicKey: publicKey, SigningPublicKey: signingPublicKey})
	if !errors.Is(err, ErrDeviceIDConflict) {
		t.Fatalf("err = %v, want ErrDeviceIDConflict", err)
	}
}

func TestRegisterDevice_RaceOnWrite(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey, signingPublicKey := testKeys(t)
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{}}
	writer := &fakeDeviceWriter{registerErr: errors.New("unique violation")}
	h := NewRegisterDeviceHandler(loader, writer)

	_, _, err := h.Handle(context.Background(), in.RegisterDeviceCommand{Actor: actor, DeviceID: deviceID, PublicKey: publicKey, SigningPublicKey: signingPublicKey})
	if !errors.Is(err, ErrDeviceAlreadyRegistered) {
		t.Fatalf("err = %v, want ErrDeviceAlreadyRegistered", err)
	}
}

func TestRegisterDevice_InvalidKeys(t *testing.T) {
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{}}
	h := NewRegisterDeviceHandler(loader, &fakeDeviceWriter{})

	_, _, err := h.Handle(context.Background(), in.RegisterDeviceCommand{Actor: uuid.Must(uuid.NewV7()), DeviceID: uuid.Must(uuid.NewV7()), PublicKey: make([]byte, 31), SigningPublicKey: make([]byte, ed25519.PublicKeySize)})
	if !errors.Is(err, domaine2ee.ErrInvalidKey) {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
}
