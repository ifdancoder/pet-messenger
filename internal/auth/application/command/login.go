package command

import (
	"context"
	"errors"

	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
	domainuser "pet-messenger/internal/user/domain"
)

// ErrInvalidCredentials: Login was called with a username/password pair
// that doesn't match an account.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

type loginHandler struct {
	users    out.UserStore
	hasher   out.PasswordHasher
	sessions out.SessionStore
	tokens   out.TokenIssuer
	refresh  out.RefreshTokenGenerator
}

// NewLoginHandler wires the Login use case.
func NewLoginHandler(users out.UserStore, hasher out.PasswordHasher, sessions out.SessionStore, tokens out.TokenIssuer, refresh out.RefreshTokenGenerator) in.LoginUseCase {
	return &loginHandler{users: users, hasher: hasher, sessions: sessions, tokens: tokens, refresh: refresh}
}

// Handle verifies cmd's credentials and issues a new session.
func (h *loginHandler) Handle(ctx context.Context, cmd in.LoginCommand) (*in.Result, error) {
	u, err := h.users.GetByUsername(ctx, cmd.Username)
	if err != nil {
		if errors.Is(err, domainuser.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := h.hasher.Verify(u.PasswordHash, cmd.Password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return issueSession(ctx, h.sessions, h.tokens, h.refresh, u, cmd.DeviceInfo)
}
