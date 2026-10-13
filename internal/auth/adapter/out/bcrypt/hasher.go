// internal/auth/adapter/out/bcrypt/hasher.go

// Package bcrypt is the auth context's bcrypt-backed implementation of
// out.PasswordHasher.
package bcrypt

import (
	"golang.org/x/crypto/bcrypt"

	"pet-messenger/internal/auth/application/port/out"
)

// Hasher implements out.PasswordHasher.
type Hasher struct{}

// NewHasher constructs the port directly — the same convention
// internal/infrastructure/nats.NewPublisher already uses — so no separate
// fx bridge function is needed.
func NewHasher() out.PasswordHasher {
	return &Hasher{}
}

func (h *Hasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (h *Hasher) Verify(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
