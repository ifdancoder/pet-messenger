// internal/auth/application/command/refresh_test.go
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

func newRefreshHandlers() (in.RegisterUseCase, in.RefreshUseCase, *fakeUserStore, *fakeSessionStore) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()
	refresh := &fakeRefreshGenerator{}
	register := command.NewRegisterHandler(users, fakeHasher{}, sessions, fakeTokenIssuer{}, refresh)
	refreshHandler := command.NewRefreshHandler(sessions, users, fakeTokenIssuer{}, refresh, out.RefreshTTL(30*24*time.Hour))
	return register, refreshHandler, users, sessions
}

func TestRefresh_Success(t *testing.T) {
	register, refresh, _, sessions := newRefreshHandlers()
	ctx := context.Background()
	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.User.ID != reg.User.ID {
		t.Errorf("User.ID = %v, want %v", result.User.ID, reg.User.ID)
	}
	if result.RefreshToken == reg.RefreshToken {
		t.Error("expected Refresh to rotate to a new refresh token")
	}
	if got := sessions.countActiveForUser(result.User.ID); got != 1 {
		t.Errorf("active sessions after rotation = %d, want 1", got)
	}
}

func TestRefresh_OldTokenRejectedAfterRotation(t *testing.T) {
	register, refresh, _, _ := newRefreshHandlers()
	ctx := context.Background()
	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken}); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken}); !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken reusing a rotated-away token, got %v", err)
	}
}

func TestRefresh_ReuseRevokesWholeChain(t *testing.T) {
	register, refresh, _, sessions := newRefreshHandlers()
	ctx := context.Background()
	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	rotated, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken})
	if err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	// reuse of the already-rotated-away token: simulates a stolen token being replayed
	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken}); !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken on reuse, got %v", err)
	}

	// the legitimate, freshly rotated token must now be revoked too
	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: rotated.RefreshToken}); !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected the rotated token to be revoked after reuse was detected, got %v", err)
	}
	if got := sessions.countActiveForUser(reg.User.ID); got != 0 {
		t.Errorf("active sessions after reuse detection = %d, want 0", got)
	}
}

func TestRefresh_ExpiredToken(t *testing.T) {
	register, refresh, _, sessions := newRefreshHandlers()
	ctx := context.Background()
	reg, err := register.Handle(ctx, in.RegisterCommand{Username: "alice_01", Password: "password123", DisplayName: "Alice"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	sessions.mu.Lock()
	for _, s := range sessions.byHash {
		if s.UserID == reg.User.ID {
			s.CreatedAt = time.Now().Add(-31 * 24 * time.Hour)
		}
	}
	sessions.mu.Unlock()

	if _, err := refresh.Handle(ctx, in.RefreshCommand{RefreshToken: reg.RefreshToken}); !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for expired session, got %v", err)
	}
}

func TestRefresh_UnknownToken(t *testing.T) {
	_, refresh, _, _ := newRefreshHandlers()
	_, err := refresh.Handle(context.Background(), in.RefreshCommand{RefreshToken: "does-not-exist"})
	if !errors.Is(err, command.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
}
