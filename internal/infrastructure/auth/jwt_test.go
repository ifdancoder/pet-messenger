package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/infrastructure/auth"
)

// signTestToken builds a minimal access-token-shaped JWT directly, so
// these tests can control expiry independently of any real TTL and
// without depending on internal/auth/adapter/out/jwt.Issuer.
func signTestToken(t *testing.T, priv ed25519.PrivateKey, userID uuid.UUID, ttl time.Duration) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ID:        uuid.NewString(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

func TestJWTVerifier_VerifiesValidToken(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	verifier := auth.NewJWTVerifierWithKey(pub)

	userID := uuid.New()
	token := signTestToken(t, priv, userID, time.Minute)

	gotID, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if gotID != userID {
		t.Errorf("Verify() = %v, want %v", gotID, userID)
	}
}

func TestJWTVerifier_RejectsExpired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	verifier := auth.NewJWTVerifierWithKey(pub)

	token := signTestToken(t, priv, uuid.New(), -time.Minute) // already expired
	if _, err := verifier.Verify(token); err == nil {
		t.Error("expected an error verifying an expired token")
	}
}

func TestJWTVerifier_RejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	verifier := auth.NewJWTVerifierWithKey(otherPub)

	token := signTestToken(t, priv, uuid.New(), time.Minute)
	if _, err := verifier.Verify(token); err == nil {
		t.Error("expected an error verifying a token signed with a different key")
	}
}
