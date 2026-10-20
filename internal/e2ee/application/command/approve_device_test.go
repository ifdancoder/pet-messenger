// internal/e2ee/application/command/approve_device_test.go
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

func TestApproveDevice_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, signingPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	target := &domaine2ee.Device{ID: targetID, UserID: actor, Status: domaine2ee.StatusPending, Fingerprint: "target-fp"}
	approver := &domaine2ee.Device{ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey}
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{approverID: approver, targetID: target}}
	writer := &fakeDeviceWriter{}
	h := NewApproveDeviceHandler(loader, writer)

	sig := ed25519.Sign(signingPrivateKey, []byte("pet-messenger:approve-device:v1:"+approverID.String()+":"+targetID.String()+":target-fp"))
	if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: sig}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	expected := approval{Approver: approverID, Target: targetID}
	if len(writer.approved) != 1 || writer.approved[0] != expected {
		t.Fatalf("approved = %+v", writer.approved)
	}
}

func TestApproveDevice_ApproverNotTrusted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)

	cases := []struct {
		name     string
		approver *domaine2ee.Device
	}{
		{"not found", nil},
		{"wrong owner", &domaine2ee.Device{ID: approverID, UserID: uuid.Must(uuid.NewV7()), Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey}},
		{"not trusted", &domaine2ee.Device{ID: approverID, UserID: actor, Status: domaine2ee.StatusPending, SigningPublicKey: signingPublicKey}},
		{"malformed signing key", &domaine2ee.Device{ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey[:10]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			byID := map[uuid.UUID]*domaine2ee.Device{targetID: {ID: targetID, UserID: actor, Status: domaine2ee.StatusPending}}
			if tc.approver != nil {
				byID[approverID] = tc.approver
			}
			h := NewApproveDeviceHandler(&fakeDeviceLoader{byID: byID}, &fakeDeviceWriter{})
			if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: make([]byte, ed25519.SignatureSize)}); !errors.Is(err, ErrApproverNotTrusted) {
				t.Fatalf("err = %v, want ErrApproverNotTrusted", err)
			}
		})
	}
}

func TestApproveDevice_PendingDeviceNotFound(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	approver := &domaine2ee.Device{ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey}

	cases := []struct {
		name   string
		target *domaine2ee.Device
	}{
		{"not found", nil},
		{"wrong owner", &domaine2ee.Device{ID: targetID, UserID: uuid.Must(uuid.NewV7()), Status: domaine2ee.StatusPending}},
		{"already trusted", &domaine2ee.Device{ID: targetID, UserID: actor, Status: domaine2ee.StatusTrusted}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			byID := map[uuid.UUID]*domaine2ee.Device{approverID: approver}
			if tc.target != nil {
				byID[targetID] = tc.target
			}
			h := NewApproveDeviceHandler(&fakeDeviceLoader{byID: byID}, &fakeDeviceWriter{})
			if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: make([]byte, ed25519.SignatureSize)}); !errors.Is(err, ErrPendingDeviceNotFound) {
				t.Fatalf("err = %v, want ErrPendingDeviceNotFound", err)
			}
		})
	}
}

func TestApproveDevice_InvalidSignature(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{
		approverID: {ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey},
		targetID:   {ID: targetID, UserID: actor, Status: domaine2ee.StatusPending, Fingerprint: "fp"},
	}}
	h := NewApproveDeviceHandler(loader, &fakeDeviceWriter{})

	if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: make([]byte, ed25519.SignatureSize)}); !errors.Is(err, ErrInvalidApprovalSignature) {
		t.Fatalf("err = %v, want ErrInvalidApprovalSignature", err)
	}
}

func TestApproveDevice_WriterNotFound(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, signingPrivateKey, _ := ed25519.GenerateKey(rand.Reader)
	target := &domaine2ee.Device{ID: targetID, UserID: actor, Status: domaine2ee.StatusPending, Fingerprint: "fp"}
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{
		approverID: {ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey},
		targetID:   target,
	}}
	writer := &fakeDeviceWriter{approveErr: domaine2ee.ErrNotFound}
	h := NewApproveDeviceHandler(loader, writer)

	sig := ed25519.Sign(signingPrivateKey, []byte("pet-messenger:approve-device:v1:"+approverID.String()+":"+targetID.String()+":fp"))
	if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: sig}); !errors.Is(err, ErrPendingDeviceNotFound) {
		t.Fatalf("err = %v, want ErrPendingDeviceNotFound (writer's ErrNotFound maps back to it)", err)
	}
}

func TestApproveDevice_WriterOtherError(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	approverID, targetID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	signingPublicKey, signingPrivateKey, _ := ed25519.GenerateKey(rand.Reader)
	target := &domaine2ee.Device{ID: targetID, UserID: actor, Status: domaine2ee.StatusPending, Fingerprint: "fp"}
	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{
		approverID: {ID: approverID, UserID: actor, Status: domaine2ee.StatusTrusted, SigningPublicKey: signingPublicKey},
		targetID:   target,
	}}
	writer := &fakeDeviceWriter{approveErr: errors.New("db down")}
	h := NewApproveDeviceHandler(loader, writer)

	sig := ed25519.Sign(signingPrivateKey, []byte("pet-messenger:approve-device:v1:"+approverID.String()+":"+targetID.String()+":fp"))
	if err := h.Handle(context.Background(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approverID, TargetID: targetID, Signature: sig}); !errors.Is(err, ErrCannotApprove) {
		t.Fatalf("err = %v, want ErrCannotApprove", err)
	}
}
