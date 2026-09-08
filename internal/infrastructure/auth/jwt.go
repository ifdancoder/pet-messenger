package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/config"
	usecaseauth "pet-messenger/internal/usecase/auth"
)

type claims struct {
	jwt.RegisteredClaims
}

type JWTIssuer struct {
	privateKey ed25519.PrivateKey
	ttl        time.Duration
}

func NewJWTIssuerWithKey(privateKey ed25519.PrivateKey, ttl time.Duration) *JWTIssuer {
	return &JWTIssuer{privateKey: privateKey, ttl: ttl}
}

func NewJWTIssuer(cfg *config.Config) usecaseauth.TokenIssuer {
	return NewJWTIssuerWithKey(cfg.JWT.PrivateKey, cfg.JWT.AccessTTL)
}

func (j *JWTIssuer) IssueAccessToken(userID uuid.UUID) (string, time.Time, error) {
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

var ErrInvalidToken = errors.New("auth: invalid or expired token")

type JWTVerifier struct {
	publicKey ed25519.PublicKey
}

func NewJWTVerifierWithKey(publicKey ed25519.PublicKey) *JWTVerifier {
	return &JWTVerifier{publicKey: publicKey}
}

func NewJWTVerifier(cfg *config.Config) *JWTVerifier {
	return NewJWTVerifierWithKey(cfg.JWT.PublicKey)
}

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
	if c.ExpiresAt == nil {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, time.Time{}, ErrInvalidToken
	}
	return userID, c.ExpiresAt.Time, nil
}
