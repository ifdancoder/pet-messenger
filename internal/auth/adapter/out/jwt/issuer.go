// internal/auth/adapter/out/jwt/issuer.go

// Package jwt is the auth context's implementation of out.TokenIssuer: the
// issuing half of what internal/infrastructure/auth/jwt.go used to hold in
// one file. The verifying half (JWTVerifier) stays in
// internal/infrastructure/auth: it is shared, cross-cutting infrastructure
// consumed directly by ws/signaling/sfu/call/media/http-middleware, with
// zero involvement from this context's own application layer.
package jwt

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/auth/application/port/out"
	"pet-messenger/internal/config"
)

type claims struct {
	jwt.RegisteredClaims
}

// Issuer implements out.TokenIssuer.
type Issuer struct {
	privateKey ed25519.PrivateKey
	ttl        time.Duration
}

// NewIssuerWithKey constructs an Issuer from a raw key, for tests and for
// NewIssuer below.
func NewIssuerWithKey(privateKey ed25519.PrivateKey, ttl time.Duration) *Issuer {
	return &Issuer{privateKey: privateKey, ttl: ttl}
}

// NewIssuer constructs the port directly from config.
func NewIssuer(cfg *config.Config) out.TokenIssuer {
	return NewIssuerWithKey(cfg.JWT.PrivateKey, cfg.JWT.AccessTTL)
}

// IssueAccessToken signs and returns a new access token for userID, along
// with the time at which it expires.
func (j *Issuer) IssueAccessToken(userID uuid.UUID) (string, time.Time, error) {
	expiresAt := time.Now().Add(j.ttl)
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	signed, err := token.SignedString(j.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}
