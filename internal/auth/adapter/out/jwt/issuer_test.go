// internal/auth/adapter/out/jwt/issuer_test.go
package jwt_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/auth/adapter/out/jwt"
)

func TestIssuer_IssueAccessToken(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	issuer := jwt.NewIssuerWithKey(priv, time.Minute)

	userID := uuid.New()
	token, expiresAt, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}
	if until := time.Until(expiresAt); until <= 0 || until > time.Minute {
		t.Errorf("unexpected expiry: %v from now", until)
	}

	parsed, _, err := jwtlib.NewParser().ParseUnverified(token, &jwtlib.RegisteredClaims{})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	claims, ok := parsed.Claims.(*jwtlib.RegisteredClaims)
	if !ok {
		t.Fatalf("unexpected claims type %T", parsed.Claims)
	}
	if claims.Subject != userID.String() {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID.String())
	}
}
