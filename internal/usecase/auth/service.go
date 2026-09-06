// internal/usecase/auth/service.go
package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	domainauth "pet-messenger/internal/domain/auth"
	"pet-messenger/internal/domain/user"
)

var (
	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrWeakPassword        = errors.New("auth: password must be at least 8 characters")
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
)

type Service struct {
	users      user.Repository
	sessions   domainauth.Repository
	hasher     PasswordHasher
	tokens     TokenIssuer
	refresh    RefreshTokenGenerator
	refreshTTL RefreshTTL
}

func NewService(
	users user.Repository,
	sessions domainauth.Repository,
	hasher PasswordHasher,
	tokens TokenIssuer,
	refresh RefreshTokenGenerator,
	refreshTTL RefreshTTL,
) *Service {
	return &Service{
		users:      users,
		sessions:   sessions,
		hasher:     hasher,
		tokens:     tokens,
		refresh:    refresh,
		refreshTTL: refreshTTL,
	}
}

type Result struct {
	User            *user.User
	AccessToken     string
	AccessExpiresAt time.Time
	RefreshToken    string
}

func (s *Service) Register(ctx context.Context, username, password, displayName string) (*Result, error) {
	if len(password) < 8 {
		return nil, ErrWeakPassword
	}
	u, err := user.New(username, displayName)
	if err != nil {
		return nil, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = hash

	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, u, "")
}

func (s *Service) Login(ctx context.Context, username, password, deviceInfo string) (*Result, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := s.hasher.Verify(u.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(ctx, u, deviceInfo)
}

func (s *Service) issueSession(ctx context.Context, u *user.User, deviceInfo string) (*Result, error) {
	plain, hash, err := s.refresh.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	sess := domainauth.New(u.ID, hash, deviceInfo)
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	accessToken, expiresAt, err := s.tokens.IssueAccessToken(u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	return &Result{
		User:            u,
		AccessToken:     accessToken,
		AccessExpiresAt: expiresAt,
		RefreshToken:    plain,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Result, error) {
	hash := s.refresh.Hash(refreshToken)
	sess, err := s.sessions.GetByRefreshTokenHash(ctx, hash)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if sess.IsRevoked() {
		// Presenting an already-rotated-away or already-revoked token is a
		// sign of theft/replay: kill every session for this user, not just this one.
		if err := s.sessions.RevokeAllForUser(ctx, sess.UserID); err != nil {
			log.Printf("auth: failed to revoke all sessions for user %s after refresh-token reuse: %v", sess.UserID, err)
		}
		return nil, ErrInvalidRefreshToken
	}
	if time.Since(sess.CreatedAt) > time.Duration(s.refreshTTL) {
		return nil, ErrInvalidRefreshToken
	}

	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	plain, newHash, err := s.refresh.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	newSess := domainauth.New(u.ID, newHash, sess.DeviceInfo)
	if err := s.sessions.RotateSession(ctx, sess.ID, newSess); err != nil {
		return nil, err
	}

	accessToken, expiresAt, err := s.tokens.IssueAccessToken(u.ID)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	return &Result{
		User:            u,
		AccessToken:     accessToken,
		AccessExpiresAt: expiresAt,
		RefreshToken:    plain,
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := s.refresh.Hash(refreshToken)
	sess, err := s.sessions.GetByRefreshTokenHash(ctx, hash)
	if err != nil {
		return nil // unknown token: logout is idempotent, nothing to revoke
	}
	return s.sessions.Revoke(ctx, sess.ID)
}
