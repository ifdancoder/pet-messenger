// internal/auth/application/command/login_test.go
package command_test

import (
	"context"
	"errors"
	"testing"

	"pet-messenger/internal/auth/application/command"
	"pet-messenger/internal/auth/application/port/in"
)

func newLoginHandler() (in.RegisterUseCase, in.LoginUseCase) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()
	refresh := &fakeRefreshGenerator{}
	register := command.NewRegisterHandler(users, fakeHasher{}, sessions, fakeTokenIssuer{}, refresh)
	login := command.NewLoginHandler(users, fakeHasher{}, sessions, fakeTokenIssuer{}, refresh)
	return register, login
}

func TestLogin_Success(t *testing.T) {
	register, login := newLoginHandler()
	ctx := context.Background()
	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := login.Handle(ctx, in.LoginCommand{Username: "alice_01", Password: "password123", DeviceInfo: "device-a"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.User.ID != reg.User.ID {
		t.Errorf("User.ID = %v, want %v", result.User.ID, reg.User.ID)
	}
	if result.RefreshToken == reg.RefreshToken {
		t.Error("expected Login to issue a refresh token distinct from Register's")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	register, login := newLoginHandler()
	ctx := context.Background()
	if _, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := login.Handle(ctx, in.LoginCommand{Username: "alice_01", Password: "wrong-password"})
	if !errors.Is(err, command.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownUsername(t *testing.T) {
	_, login := newLoginHandler()
	_, err := login.Handle(context.Background(), in.LoginCommand{Username: "nobody", Password: "password123"})
	if !errors.Is(err, command.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
