package e2ee

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("e2ee: device not found")
	ErrNotTrusted     = errors.New("e2ee: approving device is not trusted")
	ErrInvalidKey     = errors.New("e2ee: invalid public key")
	ErrAlreadyTrusted = errors.New("e2ee: device is already trusted")
)

type Status string

const (
	StatusPending Status = "pending"
	StatusTrusted Status = "trusted"
	StatusRevoked Status = "revoked"
)

type Device struct {
	ID, UserID            uuid.UUID
	PublicKey             []byte
	Fingerprint           string
	Label                 string
	Status                Status
	ApprovedBy            *uuid.UUID
	CreatedAt             time.Time
	ApprovedAt, RevokedAt *time.Time
}

type Repository interface {
	Register(context.Context, *Device) error
	Get(context.Context, uuid.UUID) (*Device, error)
	ListForUser(context.Context, uuid.UUID) ([]*Device, error)
	ListTrusted(context.Context, uuid.UUID) ([]*Device, error)
	ListTrustedForConversation(context.Context, uuid.UUID) ([]*Device, error)
	Approve(context.Context, uuid.UUID, uuid.UUID) error
}
