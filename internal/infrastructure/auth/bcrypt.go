// internal/infrastructure/auth/bcrypt.go
package auth

import (
	"golang.org/x/crypto/bcrypt"

	usecaseauth "pet-messenger/internal/usecase/auth"
)

type BcryptHasher struct{}

func NewBcryptHasher() usecaseauth.PasswordHasher {
	return &BcryptHasher{}
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (h *BcryptHasher) Verify(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
