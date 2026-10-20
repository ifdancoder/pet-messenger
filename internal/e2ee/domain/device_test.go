// internal/e2ee/domain/device_test.go
package domain_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/e2ee/domain"
)

func TestStatusValid(t *testing.T) {
	tests := []struct {
		status domain.Status
		want   bool
	}{
		{domain.StatusPending, true},
		{domain.StatusTrusted, true},
		{domain.StatusRevoked, true},
		{domain.Status("bogus"), false},
		{domain.Status(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("Status(%q).Valid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestNewDevice(t *testing.T) {
	id, userID := uuid.New(), uuid.New()
	publicKey := make([]byte, 32)
	publicKey[0] = 1
	signingPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	d, err := domain.NewDevice(id, userID, publicKey, signingPublicKey, "my phone")
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	if d.ID != id || d.UserID != userID || d.Label != "my phone" {
		t.Errorf("device = %+v", d)
	}
	if d.Fingerprint == "" {
		t.Error("expected a non-empty fingerprint")
	}
	if d.Status != "" {
		t.Errorf("Status = %q, want unset (the repository decides it)", d.Status)
	}
}

func TestNewDevice_SameKeysSameFingerprint(t *testing.T) {
	publicKey := make([]byte, 32)
	publicKey[0] = 7
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)

	a, err := domain.NewDevice(uuid.New(), uuid.New(), publicKey, signingPublicKey, "")
	if err != nil {
		t.Fatalf("NewDevice a: %v", err)
	}
	b, err := domain.NewDevice(uuid.New(), uuid.New(), publicKey, signingPublicKey, "")
	if err != nil {
		t.Fatalf("NewDevice b: %v", err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Errorf("same keys produced different fingerprints: %q vs %q", a.Fingerprint, b.Fingerprint)
	}
}

func TestNewDevice_InvalidKeys(t *testing.T) {
	validSigning, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	validPublic := make([]byte, 32)

	tests := []struct {
		name             string
		publicKey        []byte
		signingPublicKey []byte
	}{
		{"short public key", make([]byte, 31), validSigning},
		{"long public key", make([]byte, 33), validSigning},
		{"short signing key", validPublic, make([]byte, ed25519.PublicKeySize-1)},
		{"long signing key", validPublic, make([]byte, ed25519.PublicKeySize+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.NewDevice(uuid.New(), uuid.New(), tt.publicKey, tt.signingPublicKey, ""); !errors.Is(err, domain.ErrInvalidKey) {
				t.Fatalf("err = %v, want ErrInvalidKey", err)
			}
		})
	}
}

func TestVerifyApprovalSignature(t *testing.T) {
	approverID := uuid.New()
	targetID := uuid.New()
	fingerprint := "target-fingerprint"
	signingPublicKey, signingPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	approver := &domain.Device{ID: approverID, SigningPublicKey: signingPublicKey}

	statement := []byte("pet-messenger:approve-device:v1:" + approverID.String() + ":" + targetID.String() + ":" + fingerprint)
	validSignature := ed25519.Sign(signingPrivateKey, statement)
	if !approver.VerifyApprovalSignature(targetID, fingerprint, validSignature) {
		t.Error("expected a valid signature to verify")
	}

	invalidSignature := make([]byte, ed25519.SignatureSize)
	if approver.VerifyApprovalSignature(targetID, fingerprint, invalidSignature) {
		t.Error("expected an invalid signature to fail verification")
	}

	otherTargetID := uuid.New()
	if approver.VerifyApprovalSignature(otherTargetID, fingerprint, validSignature) {
		t.Error("a signature for one target must not verify for a different target")
	}

	if approver.VerifyApprovalSignature(targetID, "wrong-fingerprint", validSignature) {
		t.Error("a signature for one fingerprint must not verify for a different fingerprint")
	}
}
