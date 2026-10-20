// Package command holds the CQRS write side of the auth context: one
// handler per use case, each implementing the matching port/in use case
// and depending only on the port/out interfaces it actually needs.
package command

import (
	"context"
	"fmt"

	"pet-messenger/internal/auth/application/port/in"
	"pet-messenger/internal/auth/application/port/out"
	"pet-messenger/internal/auth/domain"
	domainuser "pet-messenger/internal/user/domain"
)

// issueSession generates a refresh token, persists a new session for u,
// and issues an access token: the work shared by Register and Login
// after each has established which user is signing in. Refresh does not
// call this: it rotates an existing session instead of creating one, so
// it reimplements this tail inline against sessions.RotateSession.
func issueSession(ctx context.Context, sessions out.SessionStore, tokens out.TokenIssuer, refresh out.RefreshTokenGenerator, u *domainuser.User, deviceInfo string) (*in.Result, error) {
	plain, hash, err := refresh.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	sess := domain.New(u.ID, hash, deviceInfo)
	if err := sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	accessToken, expiresAt, err := tokens.IssueAccessToken(u.ID)
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
