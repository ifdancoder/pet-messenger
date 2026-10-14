// internal/pushtoken/application/command/register_push_token_test.go
package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/pushtoken/application/port/in"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

type fakePushTokenWriter struct {
	upsert func(t *domainpush.PushToken) (*domainpush.PushToken, error)
}

func (f *fakePushTokenWriter) Upsert(_ context.Context, t *domainpush.PushToken) (*domainpush.PushToken, error) {
	return f.upsert(t)
}

func TestRegisterPushToken_Success(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	var got *domainpush.PushToken
	h := NewRegisterPushTokenHandler(&fakePushTokenWriter{
		upsert: func(t *domainpush.PushToken) (*domainpush.PushToken, error) {
			got = t
			return t, nil
		},
	})
	out, err := h.Handle(context.Background(), in.RegisterPushTokenCommand{
		Actor: actor, Platform: domainpush.PlatformWeb, Token: "  endpoint  ",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got == nil || got.UserID != actor || got.Platform != domainpush.PlatformWeb || got.Token != "endpoint" {
		t.Fatalf("writer got = %+v", got)
	}
	if out != got {
		t.Fatalf("Handle must return what the writer returned")
	}
}

func TestRegisterPushToken_InvalidPlatform(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := NewRegisterPushTokenHandler(&fakePushTokenWriter{
		upsert: func(*domainpush.PushToken) (*domainpush.PushToken, error) {
			t.Fatal("writer must not be called for an invalid platform")
			return nil, nil
		},
	})
	_, err := h.Handle(context.Background(), in.RegisterPushTokenCommand{
		Actor: actor, Platform: "carrier-pigeon", Token: "x",
	})
	if !errors.Is(err, domainpush.ErrInvalidPlatform) {
		t.Fatalf("err = %v, want ErrInvalidPlatform", err)
	}
}

func TestRegisterPushToken_WriterError(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	wantErr := errors.New("boom")
	h := NewRegisterPushTokenHandler(&fakePushTokenWriter{
		upsert: func(*domainpush.PushToken) (*domainpush.PushToken, error) { return nil, wantErr },
	})
	_, err := h.Handle(context.Background(), in.RegisterPushTokenCommand{
		Actor: actor, Platform: domainpush.PlatformFCM, Token: "x",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
