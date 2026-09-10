// Package ice builds the ICE-server list handed to a client in the call join
// response: the static STUN URLs plus, when TURN is configured, a coturn
// entry with time-limited HMAC credentials (the "TURN REST API" scheme /
// coturn use-auth-secret). media-service never uses this — only the client.
package ice

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/config"
	usecasecall "pet-messenger/internal/usecase/call"
)

type Provider struct {
	stun    []string
	turnURL string
	turnTLS string
	secret  string
	ttl     time.Duration
	now     func() time.Time
}

func NewProvider(cfg *config.Config) *Provider {
	return &Provider{
		stun:    cfg.STUNURLs,
		turnURL: cfg.TURN.URL,
		turnTLS: cfg.TURN.TLSURL,
		secret:  cfg.TURN.Secret,
		ttl:     cfg.TURN.CredentialTTL,
		now:     time.Now,
	}
}

// ServersFor returns the STUN entry and, when TURN is configured, one TURN
// entry with fresh credentials scoped to userID.
func (p *Provider) ServersFor(userID uuid.UUID) []usecasecall.ICEServer {
	out := make([]usecasecall.ICEServer, 0, 2)
	if len(p.stun) > 0 {
		out = append(out, usecasecall.ICEServer{URLs: append([]string(nil), p.stun...)})
	}
	if p.turnURL == "" || p.secret == "" {
		return out
	}
	expiry := p.now().Add(p.ttl).Unix()
	username := strconv.FormatInt(expiry, 10) + ":" + userID.String()
	mac := hmac.New(sha1.New, []byte(p.secret))
	mac.Write([]byte(username))
	cred := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	urls := []string{p.turnURL}
	if p.turnTLS != "" {
		urls = append(urls, p.turnTLS)
	}
	out = append(out, usecasecall.ICEServer{URLs: urls, Username: username, Credential: cred})
	return out
}
