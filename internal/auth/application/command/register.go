package command

import (
	"context"
	"errors"
	"fmt"

	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
	domainuser "pet-messenger/internal/user/domain"
)

// ErrWeakPassword: Register was called with a password shorter than the
// minimum length.
var ErrWeakPassword = errors.New("auth: password must be at least 8 characters")

type registerHandler struct {
	users    out.UserStore
	hasher   out.PasswordHasher
	sessions out.SessionStore
	tokens   out.TokenIssuer
	refresh  out.RefreshTokenGenerator
}

// NewRegisterHandler wires the Register use case.
func NewRegisterHandler(users out.UserStore, hasher out.PasswordHasher, sessions out.SessionStore, tokens out.TokenIssuer, refresh out.RefreshTokenGenerator) in.RegisterUseCase {
	return &registerHandler{users: users, hasher: hasher, sessions: sessions, tokens: tokens, refresh: refresh}
}

// Handle creates a new account and immediately issues a session for it.
func (h *registerHandler) Handle(ctx context.Context, cmd in.RegisterCommand) (*in.Result, error) {
	if len(cmd.Password) < 8 {
		return nil, ErrWeakPassword
	}
	u, err := domainuser.New(cmd.Username, cmd.DisplayName)
	if err != nil {
		return nil, err
	}
	hash, err := h.hasher.Hash(cmd.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash

	if err := h.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return issueSession(ctx, h.sessions, h.tokens, h.refresh, u, "")
}
