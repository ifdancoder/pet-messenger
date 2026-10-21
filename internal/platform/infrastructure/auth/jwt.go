package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/platform/config"
)

type claims struct {
	jwt.RegisteredClaims
}

// ErrInvalidToken is returned by JWTVerifier when a token is malformed,
// unsigned by the expected key, expired, or otherwise fails validation.
var ErrInvalidToken = errors.New("auth: invalid or expired token")

// JWTVerifier validates access tokens issued elsewhere (see
// internal/auth/adapter/out/jwt.Issuer) against a known public key. It is
// shared, cross-cutting infrastructure consumed directly by
// ws/signaling/sfu/call/media and the HTTP middleware.
type JWTVerifier struct {
	publicKey ed25519.PublicKey
}

// NewJWTVerifierWithKey constructs a JWTVerifier from a raw public key, for
// tests and for NewJWTVerifier below.
func NewJWTVerifierWithKey(publicKey ed25519.PublicKey) *JWTVerifier {
	return &JWTVerifier{publicKey: publicKey}
}

// NewJWTVerifier constructs a JWTVerifier using the public key from cfg.
func NewJWTVerifier(cfg *config.Config) *JWTVerifier {
	return NewJWTVerifierWithKey(cfg.JWT.PublicKey)
}

// Verify validates tokenStr and returns the user ID encoded in its
// subject claim, or ErrInvalidToken if the token is invalid or expired.
func (v *JWTVerifier) Verify(tokenStr string) (uuid.UUID, error) {
	id, _, err := v.VerifyWithExpiry(tokenStr)
	return id, err
}

// VerifyWithExpiry validates tokenStr the same way Verify does and additionally
// returns the token's expiry time, so a caller can bound a long-lived
// connection to the access token's lifetime.
func (v *JWTVerifier) VerifyWithExpiry(tokenStr string) (uuid.UUID, time.Time, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.publicKey, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	c, ok := token.Claims.(*claims)
	if !ok {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	if len(c.Audience) > 0 {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	if c.ExpiresAt == nil {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	return userID, c.ExpiresAt.Time, nil
}
