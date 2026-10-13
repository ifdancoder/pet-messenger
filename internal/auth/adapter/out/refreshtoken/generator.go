// internal/auth/adapter/out/refreshtoken/generator.go

// Package refreshtoken is the auth context's implementation of
// out.RefreshTokenGenerator.
package refreshtoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"pet-messenger/internal/auth/application/port/out"
)

// Generator implements out.RefreshTokenGenerator.
type Generator struct{}

// NewGenerator constructs the port directly.
func NewGenerator() out.RefreshTokenGenerator {
	return &Generator{}
}

// Generate produces a new random refresh token and its hash.
func (g *Generator) Generate() (plain string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, g.Hash(plain), nil
}

// Hash deterministically hashes plain the same way Generate's returned hash does.
func (g *Generator) Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
