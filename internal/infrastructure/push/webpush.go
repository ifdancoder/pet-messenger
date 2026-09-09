package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"pet-messenger/internal/domain/notification"
	"pet-messenger/internal/domain/pushtoken"
)

// WebPush sends a notification to a browser Push API subscription using VAPID
// (RFC 8292) auth and aes128gcm content encryption (RFC 8291); both are handled
// by webpush-go. tok.Token is the JSON PushSubscription the browser handed the
// client.
type WebPush struct {
	options *webpushlib.Options
}

func NewWebPush(vapidPublic, vapidPrivate, vapidSubject string) *WebPush {
	return &WebPush{options: &webpushlib.Options{
		VAPIDPublicKey:  vapidPublic,
		VAPIDPrivateKey: vapidPrivate,
		Subscriber:      vapidSubject,
		TTL:             86400,
	}}
}

func (w *WebPush) Send(ctx context.Context, tok *pushtoken.PushToken, n *notification.Notification) error {
	var sub webpushlib.Subscription
	if err := json.Unmarshal([]byte(tok.Token), &sub); err != nil {
		return fmt.Errorf("%w: bad web-push subscription json", ErrTokenGone)
	}
	resp, err := webpushlib.SendNotificationWithContext(ctx, Body(n), &sub, w.options)
	if err != nil {
		return fmt.Errorf("web-push send: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrTokenGone
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	default:
		return fmt.Errorf("web-push send: status %d", resp.StatusCode)
	}
}
