// internal/usecase/auth/service_test.go
package auth_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domainauth "pet-messenger/internal/domain/auth"
	"pet-messenger/internal/usecase/auth"
	domainuser "pet-messenger/internal/user/domain"
)

type fakeUserRepo struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*domainuser.User
	byName map[string]*domainuser.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*domainuser.User{}, byName: map[string]*domainuser.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *domainuser.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[u.Username]; exists {
		return domainuser.ErrUsernameTaken
	}
	cp := *u
	r.byID[u.ID] = &cp
	r.byName[u.Username] = &cp
	return nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	return u, nil
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byName[username]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	return u, nil
}

func (r *fakeUserRepo) Update(_ context.Context, _ *domainuser.User) error { return nil }

type fakeSessionRepo struct {
	mu     sync.Mutex
	byHash map[string]*domainauth.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byHash: map[string]*domainauth.Session{}}
}

func (r *fakeSessionRepo) Create(_ context.Context, s *domainauth.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.byHash[s.RefreshTokenHash] = &cp
	return nil
}

func (r *fakeSessionRepo) GetByRefreshTokenHash(_ context.Context, hash string) (*domainauth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byHash[hash]
	if !ok {
		return nil, domainauth.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeSessionRepo) RotateSession(_ context.Context, oldSessionID uuid.UUID, newSession *domainauth.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for hash, s := range r.byHash {
		if s.ID == oldSessionID {
			now := time.Now().UTC()
			s.RevokedAt = &now
			r.byHash[hash] = s
		}
	}
	cp := *newSession
	r.byHash[newSession.RefreshTokenHash] = &cp
	return nil
}

func (r *fakeSessionRepo) Revoke(_ context.Context, sessionID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.byHash {
		if s.ID == sessionID {
			now := time.Now().UTC()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeSessionRepo) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.byHash {
		if s.UserID == userID {
			now := time.Now().UTC()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeSessionRepo) countActiveForUser(userID uuid.UUID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.byHash {
		if s.UserID == userID && !s.IsRevoked() {
			n++
		}
	}
	return n
}

type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }
func (fakeHasher) Verify(hash, password string) error {
	if hash != "hashed:"+password {
		return errors.New("mismatch")
	}
	return nil
}

type fakeTokenIssuer struct{}

func (fakeTokenIssuer) IssueAccessToken(userID uuid.UUID) (string, time.Time, error) {
	return "access-for-" + userID.String(), time.Now().Add(15 * time.Minute), nil
}

type fakeRefreshGenerator struct {
	mu      sync.Mutex
	counter int
}

func (g *fakeRefreshGenerator) Generate() (string, string, error) {
	g.mu.Lock()
	g.counter++
	n := g.counter
	g.mu.Unlock()
	plain := fmt.Sprintf("refresh-plain-%d", n)
	return plain, g.Hash(plain), nil
}

func (g *fakeRefreshGenerator) Hash(plain string) string {
	return "hash:" + plain
}

func newTestService() (*auth.Service, *fakeUserRepo, *fakeSessionRepo) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	svc := auth.NewService(users, sessions, fakeHasher{}, fakeTokenIssuer{}, &fakeRefreshGenerator{}, auth.RefreshTTL(30*24*time.Hour))
	return svc, users, sessions
}

func TestService_Register(t *testing.T) {
	svc, _, sessions := newTestService()

	result, err := svc.Register(context.Background(), "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
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

func TestService_Register_WeakPassword(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Register(context.Background(), "alice_01", "short", "Alice")
	if !errors.Is(err, auth.ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

func TestService_Register_InvalidUsername(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Register(context.Background(), "a", "password123", "")
	if !errors.Is(err, domainuser.ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got %v", err)
	}
}

func TestService_Register_DuplicateUsername(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	if _, err := svc.Register(ctx, "alice_01", "password123", "Alice"); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if _, err := svc.Register(ctx, "alice_01", "password123", "Alice 2"); !errors.Is(err, domainuser.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestService_Login(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := svc.Login(ctx, "alice_01", "password123", "device-a")
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

func TestService_Login_WrongPassword(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	if _, err := svc.Register(ctx, "alice_01", "password123", "Alice"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := svc.Login(ctx, "alice_01", "wrong-password", "")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Login_UnknownUsername(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Login(context.Background(), "nobody", "password123", "")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Refresh(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := svc.Refresh(ctx, reg.RefreshToken)
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

func TestService_Refresh_OldTokenRejectedAfterRotation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.Refresh(ctx, reg.RefreshToken); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken reusing a rotated-away token, got %v", err)
	}
}

func TestService_Refresh_ReuseRevokesWholeChain(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	rotated, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	// reuse of the already-rotated-away token: simulates a stolen token being replayed
	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken on reuse, got %v", err)
	}

	// the legitimate, freshly rotated token must now be revoked too
	if _, err := svc.Refresh(ctx, rotated.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected the rotated token to be revoked after reuse was detected, got %v", err)
	}
	if got := sessions.countActiveForUser(reg.User.ID); got != 0 {
		t.Errorf("active sessions after reuse detection = %d, want 0", got)
	}
}

func TestService_Refresh_ExpiredToken(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
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

	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for expired session, got %v", err)
	}
}

func TestService_Refresh_UnknownToken(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Refresh(context.Background(), "does-not-exist")
	if !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
}

func TestService_Logout(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice_01", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.Logout(ctx, reg.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if got := sessions.countActiveForUser(reg.User.ID); got != 0 {
		t.Errorf("active sessions after logout = %d, want 0", got)
	}

	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("expected Refresh to fail after logout, got %v", err)
	}
}

func TestService_Logout_UnknownTokenIsIdempotent(t *testing.T) {
	svc, _, _ := newTestService()
	if err := svc.Logout(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("expected Logout to be idempotent for unknown tokens, got %v", err)
	}
}
