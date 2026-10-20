// internal/e2ee/domain/device.go

// Package domain holds the Device and KeyEnvelope aggregates: end-to-end
// encryption device identities (public keys, trust status) and the opaque,
// per-recipient key packages that make an encrypted message or call key
// available to every trusted device without the server ever seeing
// plaintext. It is the Entities layer (Clean Architecture) / domain core
// (Hexagonal) for the e2ee bounded context: no dependency on anything
// outside the standard library, uuid, and the two public-key primitives
// (ed25519 signature verification, sha256 fingerprinting) that are pure
// functions of already-validated byte slices.
package domain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Domain errors.
var (
	ErrNotFound       = errors.New("e2ee: device not found")
	ErrNotTrusted     = errors.New("e2ee: approving device is not trusted")
	ErrInvalidKey     = errors.New("e2ee: invalid public key")
	ErrAlreadyTrusted = errors.New("e2ee: device is already trusted")
)

// Status is a device's trust lifecycle stage.
type Status string

// Device statuses.
const (
	StatusPending Status = "pending"
	StatusTrusted Status = "trusted"
	StatusRevoked Status = "revoked"
)

// Valid reports whether s is one of the three known statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusTrusted, StatusRevoked:
		return true
	default:
		return false
	}
}

// Device is one end-to-end encryption identity: a device's X25519 key
// agreement public key plus its ed25519 signing public key, and the trust
// chain that let it join the account's set of trusted devices.
// Fingerprint is derived once, at construction, from both public keys, and
// never changes afterward.
type Device struct {
	ID, UserID            uuid.UUID
	PublicKey             []byte
	SigningPublicKey      []byte
	Fingerprint           string
	Label                 string
	Status                Status
	ApprovedBy            *uuid.UUID
	CreatedAt             time.Time
	ApprovedAt, RevokedAt *time.Time
}

// NewDevice validates the two key lengths (32 bytes for the X25519 public
// key, ed25519.PublicKeySize for the signing key) and derives the device's
// fingerprint from both. Status and CreatedAt are left unset: whether a
// device is the account's first, and therefore auto-trusted, is a
// database-serialized decision the persistence adapter makes under a row
// lock, not something this constructor can know.
func NewDevice(id, userID uuid.UUID, publicKey, signingPublicKey []byte, label string) (*Device, error) {
	if len(publicKey) != 32 {
		return nil, ErrInvalidKey
	}
	if len(signingPublicKey) != ed25519.PublicKeySize {
		return nil, ErrInvalidKey
	}
	fingerprintMaterial := append(append(make([]byte, 0, len(publicKey)+len(signingPublicKey)), publicKey...), signingPublicKey...)
	sum := sha256.Sum256(fingerprintMaterial)
	return &Device{
		ID:               id,
		UserID:           userID,
		PublicKey:        publicKey,
		SigningPublicKey: signingPublicKey,
		Fingerprint:      hex.EncodeToString(sum[:]),
		Label:            label,
	}, nil
}

// approvalStatement is the exact byte sequence an approving device signs to
// vouch for a pending device. The format is versioned (v1) so a future
// change can be introduced without breaking already-issued signatures.
func approvalStatement(approverID, targetID uuid.UUID, targetFingerprint string) []byte {
	return []byte("pet-messenger:approve-device:v1:" + approverID.String() + ":" + targetID.String() + ":" + targetFingerprint)
}

// VerifyApprovalSignature reports whether signature is a valid ed25519
// signature, under d's own signing public key, of the approval statement
// for targetID/targetFingerprint. d is the approving (trusted) device;
// callers must separately confirm d.Status == StatusTrusted and that d's
// SigningPublicKey has the right length before trusting a true result,
// since this method only checks the cryptographic signature itself.
func (d *Device) VerifyApprovalSignature(targetID uuid.UUID, targetFingerprint string, signature []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(d.SigningPublicKey), approvalStatement(d.ID, targetID, targetFingerprint), signature)
}

// KeyEnvelope is one opaque, per-recipient key package: OwnerDeviceID's
// copy of a message or call key, wrapped so only RecipientDeviceID can open
// it. The server stores and serves the ciphertext without ever being able
// to decrypt it.
type KeyEnvelope struct {
	OwnerDeviceID     uuid.UUID
	RecipientDeviceID uuid.UUID
	Scope             string
	Ciphertext        []byte
	CreatedAt         time.Time
}
