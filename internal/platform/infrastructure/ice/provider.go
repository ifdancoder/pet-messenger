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

	callout "pet-messenger/internal/call/application/port/out"
	"pet-messenger/internal/platform/config"
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
func (p *Provider) ServersFor(userID uuid.UUID) []callout.ICEServer {
	out := make([]callout.ICEServer, 0, 2)
	if len(p.stun) > 0 {
		out = append(out, callout.ICEServer{URLs: append([]string(nil), p.stun...)})
	}
	if p.turnURL == "" || p.secret == "" {
		return out
	}
	expiry := p.now().Add(p.ttl).Unix()
	username := strconv.FormatInt(expiry, 10) + ":" + userID.String()
	mac := hmac.New(sha1.New, []byte(p.secret))
	mac.Write([]byte(username))
	cred := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// A TCP-transport variant is offered alongside the default UDP TURN URL:
	// UDP is coturn's normal, lower-overhead path, but some networks (a VPN
	// client's own WebRTC/leak-protection filtering in particular, which
	// commonly targets UDP specifically since that's the classic STUN/TURN
	// IP-leak vector) block outbound UDP from the browser process itself
	// without blocking TCP. Offering both lets ICE fall back to whichever
	// one the client's network actually allows, instead of failing outright
	// when only one direction is viable.
	//
	// Each variant gets its own ICEServer entry rather than sharing one
	// "urls" array: Firefox was observed silently discarding an entire
	// RTCIceServer whose urls array mixed a plain "turn:" URL with a
	// "?transport=tcp" one — it never attempted TURN over either transport,
	// while Chromium-based browsers handled the same array fine. Separate
	// entries are also the conventional way real TURN deployments (Twilio's
	// NTS docs, for one) advertise multiple transports, so this sidesteps
	// the ambiguity entirely instead of relying on multi-URL handling.
	out = append(out, callout.ICEServer{URLs: []string{p.turnURL}, Username: username, Credential: cred})
	out = append(out, callout.ICEServer{URLs: []string{turnURLWithTCP(p.turnURL)}, Username: username, Credential: cred})
	if p.turnTLS != "" {
		out = append(out, callout.ICEServer{URLs: []string{p.turnTLS}, Username: username, Credential: cred})
	}
	return out
}

// turnURLWithTCP appends coturn's own documented query-string transport
// selector to a plain "turn:host:port" URL. A "turns:" (TLS) URL is already
// TCP-only and never needs this.
func turnURLWithTCP(turnURL string) string {
	return turnURL + "?transport=tcp"
}
