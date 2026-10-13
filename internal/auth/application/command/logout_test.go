// internal/auth/application/command/logout_test.go
package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pet-messenger/internal/auth/application/command"
	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
)

func TestLogout_Success(t *testing.T) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()
	register := command.NewRegisterHandler(users, fakeHasher{}, sessions, fakeTokenIssuer{}, &fakeRefreshGenerator{})
	logout := command.NewLogoutHandler(sessions, &fakeRefreshGenerator{})
	refresh := command.NewRefreshHandler(sessions, users, fakeTokenIssuer{}, &fakeRefreshGenerator{}, out.RefreshTTL(30*24*time.Hour))
	ctx := context.Background()

	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Logout must hash with the SAME generator Register used to create the
	// session, so build it with a generator whose Hash is deterministic
	// and shared — the fakeRefreshGenerator.Hash method is pure (no
	// internal state), so a fresh instance hashes identically.
	if err := logout.Handle(ctx, in.LogoutCommand{RefreshToken: reg.RefreshToken}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if got := sessions.countActiveForUser(reg.User.ID); got != 0 {
		t.Errorf("active sessions after logout = %d, want 0", got)
	}

	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken}); !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected Refresh to fail after logout, got %v", err)
	}
}

func TestLogout_UnknownTokenIsIdempotent(t *testing.T) {
	sessions := newFakeSessionStore()
	logout := command.NewLogoutHandler(sessions, &fakeRefreshGenerator{})
	if err := logout.Handle(context.Background(), in.LogoutCommand{RefreshToken: "does-not-exist"}); err != nil {
		t.Fatalf("expected Logout to be idempotent for unknown tokens, got %v", err)
	}
}
