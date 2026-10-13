package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/config"
	"pet-messenger/internal/domain/call"
)

func testCfg(t *testing.T) *config.Config {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return &config.Config{
		JWT:   config.JWTConfig{PrivateKey: priv, PublicKey: pub, AccessTTL: 15 * time.Minute},
		Media: config.MediaConfig{JoinTicketTTL: 60 * time.Second},
	}
}

func TestJoinTicket_RoundTrip(t *testing.T) {
	cfg := testCfg(t)
	iss := NewJoinTicketIssuer(cfg)
	ver := NewJoinTicketVerifier(cfg)

	callID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	tok, exp, err := iss.Issue(callID, userID, call.RoleViewer, "node-7")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if time.Until(exp) > 61*time.Second || time.Until(exp) < 55*time.Second {
		t.Fatalf("exp out of range: %v", time.Until(exp))
	}

	claims, err := ver.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.CallID != callID || claims.UserID != userID || claims.Role != call.RoleViewer || claims.NodeID != "node-7" {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestJoinTicket_RejectsAccessToken(t *testing.T) {
	cfg := testCfg(t)
	// an access token from the normal issuer has no "media" audience. Built
	// directly here (rather than via internal/auth/adapter/out/jwt.Issuer)
	// so this shared-infrastructure package's tests do not depend on a
	// specific bounded context's adapter.
	accessClaims := jwt.RegisteredClaims{
		Subject:   uuid.Must(uuid.NewV7()).String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ID:        uuid.NewString(),
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessClaims).SignedString(cfg.JWT.PrivateKey)
	if err != nil {
		t.Fatalf("sign access token: %v", err)
	}
	if _, err := NewJoinTicketVerifier(cfg).Verify(access); !errors.Is(err, ErrInvalidJoinTicket) {
		t.Fatalf("access token accepted as join-ticket: %v", err)
	}
	// and a join-ticket is not a valid access token
	jt, _, _ := NewJoinTicketIssuer(cfg).Issue(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), call.RolePublisher, "n")
	if _, err := NewJWTVerifierWithKey(cfg.JWT.PublicKey).Verify(jt); err == nil {
		t.Fatal("join-ticket accepted as access token")
	}
}

func TestJoinTicket_Expired(t *testing.T) {
	cfg := testCfg(t)
	cfg.Media.JoinTicketTTL = -time.Second
	tok, _, _ := NewJoinTicketIssuer(cfg).Issue(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), call.RolePublisher, "n")
	if _, err := NewJoinTicketVerifier(cfg).Verify(tok); !errors.Is(err, ErrInvalidJoinTicket) {
		t.Fatalf("expired ticket accepted: %v", err)
	}
}

func TestJoinTicket_BadSignature(t *testing.T) {
	cfg := testCfg(t)
	tok, _, _ := NewJoinTicketIssuer(cfg).Issue(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), call.RolePublisher, "n")

	other := testCfg(t) // different key pair
	if _, err := NewJoinTicketVerifier(other).Verify(tok); !errors.Is(err, ErrInvalidJoinTicket) {
		t.Fatalf("ticket verified under wrong key: %v", err)
	}
}
