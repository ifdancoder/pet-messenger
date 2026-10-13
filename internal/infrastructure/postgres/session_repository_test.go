// internal/infrastructure/postgres/session_repository_test.go
package postgres

import (
	"context"
	"errors"
	"testing"

	domainauth "pet-messenger/internal/domain/auth"
	domainuser "pet-messenger/internal/user/domain"
)

func createTestUser(t *testing.T) *domainuser.User {
	t.Helper()
	u, err := domainuser.New(randomUsername(), "Test User")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
	); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u
}

func TestSessionRepository_CreateAndGetByHash(t *testing.T) {
	repo := NewSessionRepository(testDB)
	u := createTestUser(t)

	sess := domainauth.New(u.ID, "hash-create-1", "device-a")
	if err := repo.Create(context.Background(), sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByRefreshTokenHash(context.Background(), "hash-create-1")
	if err != nil {
		t.Fatalf("GetByRefreshTokenHash: %v", err)
	}
	if got.UserID != u.ID {
		t.Errorf("UserID = %v, want %v", got.UserID, u.ID)
	}
	if got.IsRevoked() {
		t.Error("expected new session to not be revoked")
	}
}

func TestSessionRepository_GetByRefreshTokenHash_NotFound(t *testing.T) {
	repo := NewSessionRepository(testDB)
	_, err := repo.GetByRefreshTokenHash(context.Background(), "does-not-exist")
	if !errors.Is(err, domainauth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_RotateSession(t *testing.T) {
	repo := NewSessionRepository(testDB)
	u := createTestUser(t)
	ctx := context.Background()

	old := domainauth.New(u.ID, "hash-old", "device-a")
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newSess := domainauth.New(u.ID, "hash-new", "device-a")
	if err := repo.RotateSession(ctx, old.ID, newSess); err != nil {
		t.Fatalf("RotateSession: %v", err)
	}

	rotatedOld, err := repo.GetByRefreshTokenHash(ctx, "hash-old")
	if err != nil {
		t.Fatalf("GetByRefreshTokenHash(old): %v", err)
	}
	if !rotatedOld.IsRevoked() {
		t.Error("expected old session to be revoked after rotation")
	}

	fresh, err := repo.GetByRefreshTokenHash(ctx, "hash-new")
	if err != nil {
		t.Fatalf("GetByRefreshTokenHash(new): %v", err)
	}
	if fresh.IsRevoked() {
		t.Error("expected the newly rotated-in session to not be revoked")
	}
}

func TestSessionRepository_RevokeAllForUser(t *testing.T) {
	repo := NewSessionRepository(testDB)
	u := createTestUser(t)
	ctx := context.Background()

	s1 := domainauth.New(u.ID, "hash-multi-a", "device-a")
	s2 := domainauth.New(u.ID, "hash-multi-b", "device-b")
	if err := repo.Create(ctx, s1); err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	if err := repo.Create(ctx, s2); err != nil {
		t.Fatalf("Create s2: %v", err)
	}

	if err := repo.RevokeAllForUser(ctx, u.ID); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}

	got1, err := repo.GetByRefreshTokenHash(ctx, "hash-multi-a")
	if err != nil {
		t.Fatalf("GetByRefreshTokenHash(a): %v", err)
	}
	got2, err := repo.GetByRefreshTokenHash(ctx, "hash-multi-b")
	if err != nil {
		t.Fatalf("GetByRefreshTokenHash(b): %v", err)
	}
	if !got1.IsRevoked() || !got2.IsRevoked() {
		t.Error("expected both sessions to be revoked")
	}
}
