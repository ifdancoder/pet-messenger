// Package push delivers a notification.Notification to a single registered
// device. Pusher implementations are per-platform; MultiPusher routes by the
// token's Platform. A platform whose credentials are not configured is not
// registered and yields ErrProviderNotConfigured (the worker logs it and
// moves on — the persisted notification row is the source of truth).
package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"pet-messenger/internal/config"
	"pet-messenger/internal/domain/notification"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

var (
	// ErrTokenGone: the provider reports the token is permanently invalid
	// (HTTP 404/410, FCM UNREGISTERED / bad-token INVALID_ARGUMENT). The
	// caller deletes the token.
	ErrTokenGone = errors.New("push: device token is no longer valid")
	// ErrProviderNotConfigured: no provider is registered for the token's
	// platform.
	ErrProviderNotConfigured = errors.New("push: no provider configured for this platform")
)

type Pusher interface {
	Send(ctx context.Context, tok *domainpush.PushToken, n *notification.Notification) error
}

type MultiPusher struct {
	byPlatform map[domainpush.Platform]Pusher
}

func (m *MultiPusher) Send(ctx context.Context, tok *domainpush.PushToken, n *notification.Notification) error {
	p, ok := m.byPlatform[tok.Platform]
	if !ok {
		return ErrProviderNotConfigured
	}
	return p.Send(ctx, tok, n)
}

// NewMultiPusher registers a provider per platform when its config is present.
func NewMultiPusher(ctx context.Context, cfg config.PushConfig) (*MultiPusher, error) {
	m := &MultiPusher{byPlatform: map[domainpush.Platform]Pusher{
		domainpush.PlatformAPNS: apnsStub{},
	}}
	if cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "" && cfg.VAPIDSubject != "" {
		m.byPlatform[domainpush.PlatformWeb] = NewWebPush(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject)
		log.Printf("push: web-push provider registered")
	}
	if cfg.FCMProjectID != "" && cfg.FCMCredentialsJSON != "" {
		fcm, err := NewFCM(ctx, cfg.FCMProjectID, []byte(cfg.FCMCredentialsJSON), "")
		if err != nil {
			return nil, fmt.Errorf("configure FCM: %w", err)
		}
		m.byPlatform[domainpush.PlatformFCM] = fcm
		log.Printf("push: FCM provider registered (project %s)", cfg.FCMProjectID)
	}
	return m, nil
}

// Body is the JSON a client receives in a device push / the WS notification
// envelope's "notification" field.
func Body(n *notification.Notification) []byte {
	b, _ := json.Marshal(struct {
		ID        string          `json:"id"`
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt string          `json:"created_at"`
	}{n.ID.String(), string(n.Kind), n.Payload, n.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")})
	return b
}
