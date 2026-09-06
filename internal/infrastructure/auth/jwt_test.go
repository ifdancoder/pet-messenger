package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/infrastructure/auth"
)

func TestJWTIssuerAndVerifier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	issuer := auth.NewJWTIssuerWithKey(priv, time.Minute)
	verifier := auth.NewJWTVerifierWithKey(pub)

	userID := uuid.New()
	token, expiresAt, err := issuer.IssueAccessToken(userID)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if until := time.Until(expiresAt); until <= 0 || until > time.Minute {
		t.Errorf("unexpected expiry: %v from now", until)
	}

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
	issuer := auth.NewJWTIssuerWithKey(priv, -time.Minute) // already expired
	verifier := auth.NewJWTVerifierWithKey(pub)

	token, _, err := issuer.IssueAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if _, err := verifier.Verify(token); err == nil {
		t.Error("expected an error verifying an expired token")
	}
}

func TestJWTVerifier_RejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)

	issuer := auth.NewJWTIssuerWithKey(priv, time.Minute)
	verifier := auth.NewJWTVerifierWithKey(otherPub)

	token, _, err := issuer.IssueAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if _, err := verifier.Verify(token); err == nil {
		t.Error("expected an error verifying a token signed with a different key")
	}
}
