package command_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/auth/domain"
	domainuser "pet-messenger/internal/user/domain"
)

// fakeUserStore is an in-memory out.UserStore.
type fakeUserStore struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*domainuser.User
	byName map[string]*domainuser.User
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{byID: map[uuid.UUID]*domainuser.User{}, byName: map[string]*domainuser.User{}}
}

func (r *fakeUserStore) Create(_ context.Context, u *domainuser.User) error {
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

func (r *fakeUserStore) GetByID(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	return u, nil
}

func (r *fakeUserStore) GetByUsername(_ context.Context, username string) (*domainuser.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byName[username]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	return u, nil
}

// fakeSessionStore is an in-memory out.SessionStore.
type fakeSessionStore struct {
	mu     sync.Mutex
	byHash map[string]*domain.Session
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{byHash: map[string]*domain.Session{}}
}

func (r *fakeSessionStore) Create(_ context.Context, s *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.byHash[s.RefreshTokenHash] = &cp
	return nil
}

func (r *fakeSessionStore) GetByRefreshTokenHash(_ context.Context, hash string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byHash[hash]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeSessionStore) RotateSession(_ context.Context, oldSessionID uuid.UUID, newSession *domain.Session) error {
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

func (r *fakeSessionStore) Revoke(_ context.Context, sessionID uuid.UUID) error {
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

func (r *fakeSessionStore) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
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

func (r *fakeSessionStore) countActiveForUser(userID uuid.UUID) int {
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
