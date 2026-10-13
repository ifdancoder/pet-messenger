package command_test

import (
	"context"
	"errors"
	"testing"

	"pet-messenger/internal/auth/application/command"
	"pet-messenger/internal/auth/application/port/in"
	domainuser "pet-messenger/internal/user/domain"
)

func newRegisterHandler() (in.RegisterUseCase, *fakeSessionStore) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()
	h := command.NewRegisterHandler(users, fakeHasher{}, sessions, fakeTokenIssuer{}, &fakeRefreshGenerator{})
	return h, sessions
}

func TestRegister_Success(t *testing.T) {
	h, sessions := newRegisterHandler()

	result, err := h.Handle(context.Background(), in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.User.Username != "alice_01" {
		t.Errorf("Username = %q, want %q", result.User.Username, "alice_01")
	}
	if result.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if result.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if got := sessions.countActiveForUser(result.User.ID); got != 1 {
		t.Errorf("active sessions = %d, want 1", got)
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	h, _ := newRegisterHandler()
	_, err := h.Handle(context.Background(), in.RegisterCommand{Username: "alice_01", Password: "short", DisplayName: "Alice"})
	if !errors.Is(err, command.ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

func TestRegister_InvalidUsername(t *testing.T) {
	h, _ := newRegisterHandler()
	_, err := h.Handle(context.Background(), in.RegisterCommand{Username: "a", Password: "password123"})
	if !errors.Is(err, domainuser.ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got %v", err)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	h, _ := newRegisterHandler()
	ctx := context.Background()
	if _, err := h.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"}); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if _, err := h.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice 2"}); !errors.Is(err, domainuser.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}
