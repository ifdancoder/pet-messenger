package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
	"pet-messenger/internal/auth/domain"
)

// ErrInvalidRefreshToken: Refresh was called with a token that is
// unknown, revoked, reused, or expired.
var ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")

type refreshHandler struct {
	sessions out.SessionStore
	users    out.UserStore
	tokens   out.TokenIssuer
	refresh  out.RefreshTokenGenerator
	ttl      out.RefreshTTL
}

// NewRefreshHandler wires the Refresh use case.
func NewRefreshHandler(sessions out.SessionStore, users out.UserStore, tokens out.TokenIssuer, refresh out.RefreshTokenGenerator, ttl out.RefreshTTL) in.RefreshUseCase {
	return &refreshHandler{sessions: sessions, users: users, tokens: tokens, refresh: refresh, ttl: ttl}
}

// Handle rotates cmd's refresh token and issues a new access token. A
// token presented after it was already rotated away or revoked is treated
// as theft/replay: every session for that user is revoked, not just this one.
func (h *refreshHandler) Handle(ctx context.Context, cmd in.RefreshCommand) (*in.Result, error) {
	hash := h.refresh.Hash(cmd.RefreshToken)
	sess, err := h.sessions.GetByRefreshTokenHash(ctx, hash)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if sess.IsRevoked() {
		if err := h.sessions.RevokeAllForUser(ctx, sess.UserID); err != nil {
			log.Printf("auth: failed to revoke all sessions for user %s after refresh-token reuse: %v", sess.UserID, err)
		}
		return nil, ErrInvalidRefreshToken
	}
	if time.Since(sess.CreatedAt) > time.Duration(h.ttl) {
		return nil, ErrInvalidRefreshToken
	}

	u, err := h.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	plain, newHash, err := h.refresh.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	newSess := domain.New(u.ID, newHash, sess.DeviceInfo)
	if err := h.sessions.RotateSession(ctx, sess.ID, newSess); err != nil {
		return nil, err
	}

	accessToken, expiresAt, err := h.tokens.IssueAccessToken(u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	return &in.Result{
		User:            u,
		AccessToken:     accessToken,
		AccessExpiresAt: expiresAt,
		RefreshToken:    plain,
	}, nil
}
