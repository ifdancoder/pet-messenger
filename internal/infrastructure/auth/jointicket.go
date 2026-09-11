package auth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"pet-messenger/internal/config"
	"pet-messenger/internal/domain/call"
)

// joinTicketAudience marks a token as a media join-ticket. Access tokens carry
// no audience, so the two can never be confused even though they share the key.
const joinTicketAudience = "media"

// ErrInvalidJoinTicket is returned for any signature / expiry / audience /
// shape failure. media-service never distinguishes the reason to a client.
var ErrInvalidJoinTicket = errors.New("auth: invalid or expired join-ticket")

type joinTicketJWT struct {
	jwt.RegisteredClaims
	CallID string `json:"call_id"`
	NodeID string `json:"node_id"`
	Role   string `json:"role"`
}

// JoinTicketClaims is the verified, typed payload.
type JoinTicketClaims struct {
	CallID    uuid.UUID
	UserID    uuid.UUID
	Role      call.Role
	NodeID    string
	ExpiresAt time.Time
}

type JoinTicketIssuer struct {
	privateKey ed25519.PrivateKey
	ttl        time.Duration
}

func NewJoinTicketIssuer(cfg *config.Config) *JoinTicketIssuer {
	return &JoinTicketIssuer{privateKey: cfg.JWT.PrivateKey, ttl: cfg.Media.JoinTicketTTL}
}

func (i *JoinTicketIssuer) Issue(callID, userID uuid.UUID, role call.Role, nodeID string) (string, time.Time, error) {
	exp := time.Now().Add(i.ttl)
	claims := joinTicketJWT{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{joinTicketAudience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.NewString(),
		},
		CallID: callID.String(),
		NodeID: nodeID,
		Role:   string(role),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign join-ticket: %w", err)
	}
	return signed, exp, nil
}

type JoinTicketVerifier struct {
	publicKey ed25519.PublicKey
}

func NewJoinTicketVerifier(cfg *config.Config) *JoinTicketVerifier {
	return &JoinTicketVerifier{publicKey: cfg.JWT.PublicKey}
}

func (v *JoinTicketVerifier) Verify(token string) (JoinTicketClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &joinTicketJWT{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.publicKey, nil
	}, jwt.WithAudience(joinTicketAudience))
	if err != nil || !parsed.Valid {
		return JoinTicketClaims{}, ErrInvalidJoinTicket
	}
	c, ok := parsed.Claims.(*joinTicketJWT)
	if !ok || c.ExpiresAt == nil {
		return JoinTicketClaims{}, ErrInvalidJoinTicket
	}
	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return JoinTicketClaims{}, ErrInvalidJoinTicket
	}
	callID, err := uuid.Parse(c.CallID)
	if err != nil {
		return JoinTicketClaims{}, ErrInvalidJoinTicket
	}
	role := call.Role(c.Role)
	if !role.Valid() {
		return JoinTicketClaims{}, ErrInvalidJoinTicket
	}
	return JoinTicketClaims{
		CallID:    callID,
		UserID:    userID,
		Role:      role,
		NodeID:    c.NodeID,
		ExpiresAt: c.ExpiresAt.Time,
	}, nil
}
