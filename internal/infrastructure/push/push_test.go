package push

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/config"
	domainnotif "pet-messenger/internal/notification/domain"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

type recordingPusher struct{ calls int }

func (r *recordingPusher) Send(context.Context, *domainpush.PushToken, *domainnotif.Notification) error {
	r.calls++
	return nil
}

func mkNotif(t *testing.T) *domainnotif.Notification {
	t.Helper()
	n, err := domainnotif.New(uuid.Must(uuid.NewV7()), domainnotif.KindMessage, []byte(`{"a":1}`), "")
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMultiPusher_Routes(t *testing.T) {
	web := &recordingPusher{}
	m := &MultiPusher{byPlatform: map[domainpush.Platform]provider{domainpush.PlatformWeb: web}}
	ctx := context.Background()
	n := mkNotif(t)

	if err := m.Send(ctx, &domainpush.PushToken{Platform: domainpush.PlatformWeb, Token: "t"}, n); err != nil {
		t.Fatalf("web send: %v", err)
	}
	if web.calls != 1 {
		t.Fatalf("web calls = %d", web.calls)
	}
	if err := m.Send(ctx, &domainpush.PushToken{Platform: domainpush.PlatformFCM, Token: "t"}, n); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("fcm (unregistered) err = %v, want ErrProviderNotConfigured", err)
	}
}

func TestNewMultiPusher_RegistersFromConfig(t *testing.T) {
	m, err := NewMultiPusher(context.Background(), config.PushConfig{}) // no creds
	if err != nil {
		t.Fatalf("NewMultiPusher: %v", err)
	}
	// APNS stub always present, web/fcm absent
	if _, ok := m.byPlatform[domainpush.PlatformAPNS]; !ok {
		t.Fatal("apns stub should be registered")
	}
	if _, ok := m.byPlatform[domainpush.PlatformWeb]; ok {
		t.Fatal("web should not be registered without VAPID creds")
	}
	n := mkNotif(t)
	if err := m.Send(context.Background(), &domainpush.PushToken{Platform: domainpush.PlatformAPNS}, n); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("apns stub err = %v, want ErrProviderNotConfigured", err)
	}
}

func TestBody_Shape(t *testing.T) {
	n := mkNotif(t)
	var got struct {
		ID        string          `json:"id"`
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt string          `json:"created_at"`
	}
	if err := json.Unmarshal(Body(n), &got); err != nil {
		t.Fatalf("Body is not valid JSON: %v", err)
	}
	if got.ID != n.ID.String() {
		t.Fatalf("id = %q, want %q", got.ID, n.ID.String())
	}
	if got.Kind != string(domainnotif.KindMessage) {
		t.Fatalf("kind = %q", got.Kind)
	}
	if string(got.Payload) != `{"a":1}` {
		t.Fatalf("payload = %s", got.Payload)
	}
	if got.CreatedAt == "" {
		t.Fatal("created_at is empty")
	}
}
