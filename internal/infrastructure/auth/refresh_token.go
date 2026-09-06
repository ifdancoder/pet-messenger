// internal/infrastructure/auth/refresh_token.go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	usecaseauth "pet-messenger/internal/usecase/auth"
)

type RefreshTokenGenerator struct{}

func NewRefreshTokenGenerator() usecaseauth.RefreshTokenGenerator {
	return &RefreshTokenGenerator{}
}

func (g *RefreshTokenGenerator) Generate() (plain string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, g.Hash(plain), nil
}

func (g *RefreshTokenGenerator) Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
