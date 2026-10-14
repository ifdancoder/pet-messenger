// internal/pushtoken/application/command/unregister_push_token_test.go
package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/pushtoken/application/port/in"
)

type fakePushTokenEraser struct {
	deleteByToken func(userID uuid.UUID, token string) error
}

func (f *fakePushTokenEraser) DeleteByToken(_ context.Context, userID uuid.UUID, token string) error {
	return f.deleteByToken(userID, token)
}

func TestUnregisterPushToken_Success(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	var gotUser uuid.UUID
	var gotToken string
	h := NewUnregisterPushTokenHandler(&fakePushTokenEraser{
		deleteByToken: func(userID uuid.UUID, token string) error {
			gotUser, gotToken = userID, token
			return nil
		},
	})
	if err := h.Handle(context.Background(), in.UnregisterPushTokenCommand{Actor: actor, Token: "tok"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gotUser != actor || gotToken != "tok" {
		t.Fatalf("eraser got (user=%v token=%q)", gotUser, gotToken)
	}
}

func TestUnregisterPushToken_EraserError(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	wantErr := errors.New("boom")
	h := NewUnregisterPushTokenHandler(&fakePushTokenEraser{
		deleteByToken: func(uuid.UUID, string) error { return wantErr },
	})
	err := h.Handle(context.Background(), in.UnregisterPushTokenCommand{Actor: actor, Token: "tok"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
