package command

import (
	"context"

	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
)

type logoutHandler struct {
	sessions out.SessionStore
	refresh  out.RefreshTokenGenerator
}

// NewLogoutHandler wires the Logout use case.
func NewLogoutHandler(sessions out.SessionStore, refresh out.RefreshTokenGenerator) in.LogoutUseCase {
	return &logoutHandler{sessions: sessions, refresh: refresh}
}

// Handle revokes the session matching cmd's refresh token. Idempotent: an
// unknown token is not an error, since logout should always succeed from
// the client's point of view.
func (h *logoutHandler) Handle(ctx context.Context, cmd in.LogoutCommand) error {
	hash := h.refresh.Hash(cmd.RefreshToken)
	sess, err := h.sessions.GetByRefreshTokenHash(ctx, hash)
	if err != nil {
		return nil
	}
	return h.sessions.Revoke(ctx, sess.ID)
}
