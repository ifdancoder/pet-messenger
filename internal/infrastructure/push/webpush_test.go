package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	webpushlib "github.com/SherClockHolmes/webpush-go"

	"pet-messenger/internal/domain/pushtoken"
)

// TestWebPush_BadSubscriptionJSON exercises the JSON-unmarshal branch: an
// unparseable tok.Token means the browser subscription is gone.
func TestWebPush_BadSubscriptionJSON(t *testing.T) {
	priv, pub, err := webpushlib.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	w := NewWebPush(pub, priv, "admin@example.com")
	n := mkNotif(t)

	err = w.Send(context.Background(), &pushtoken.PushToken{Platform: pushtoken.PlatformWeb, Token: "not json"}, n)
	if !errors.Is(err, ErrTokenGone) {
		t.Fatalf("err = %v, want ErrTokenGone", err)
	}
}

// TestWebPush_StatusMapping drives a real encrypted POST at an httptest server
// and asserts the status -> error mapping: 2xx -> nil, 404/410 -> ErrTokenGone,
// anything else -> a plain (non-ErrTokenGone) error.
func TestWebPush_StatusMapping(t *testing.T) {
	priv, pub, err := webpushlib.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	subKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}

	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer srv.Close()

	sub := webpushlib.Subscription{
		Endpoint: srv.URL,
		Keys: webpushlib.Keys{
			Auth:   base64.RawURLEncoding.EncodeToString(auth),
			P256dh: base64.RawURLEncoding.EncodeToString(subKey.PublicKey().Bytes()),
		},
	}
	raw, err := json.Marshal(sub)
	if err != nil {
		t.Fatal(err)
	}

	w := NewWebPush(pub, priv, "admin@example.com")
	n := mkNotif(t)
	tok := &pushtoken.PushToken{Platform: pushtoken.PlatformWeb, Token: string(raw)}

	tests := []struct {
		name      string
		status    int
		wantErrIs error
		wantOK    bool
	}{
		{"created", http.StatusCreated, nil, true},
		{"gone", http.StatusGone, ErrTokenGone, false},
		{"not found", http.StatusNotFound, ErrTokenGone, false},
		{"server error", http.StatusInternalServerError, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status = tc.status
			err := w.Send(context.Background(), tok, n)
			switch {
			case tc.wantOK:
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
			case tc.wantErrIs != nil:
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
				}
			default:
				if err == nil || errors.Is(err, ErrTokenGone) {
					t.Fatalf("want a plain error, got %v", err)
				}
			}
		})
	}
}
