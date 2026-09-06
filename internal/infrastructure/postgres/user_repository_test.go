// internal/infrastructure/postgres/user_repository_test.go
package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/user"
)

func TestUserRepository_CreateAndGetByID(t *testing.T) {
	repo := NewUserRepository(testDB)
	ctx := context.Background()

	u, err := user.New(randomUsername(), "Alice")
	if err != nil {
		t.Fatalf("user.New: %v", err)
	}
	u.PasswordHash = "hashed"

	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Username != u.Username {
		t.Errorf("Username = %q, want %q", got.Username, u.Username)
	}
	if got.PasswordHash != u.PasswordHash {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, u.PasswordHash)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	repo := NewUserRepository(testDB)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserRepository_Create_DuplicateUsername(t *testing.T) {
	repo := NewUserRepository(testDB)
	ctx := context.Background()
	username := randomUsername()

	u1, _ := user.New(username, "First")
	u1.PasswordHash = "hashed"
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	u2, _ := user.New(username, "Second")
	u2.PasswordHash = "hashed"
	if err := repo.Create(ctx, u2); !errors.Is(err, user.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestUserRepository_GetByUsername(t *testing.T) {
	repo := NewUserRepository(testDB)
	ctx := context.Background()
	username := randomUsername()

	u, _ := user.New(username, "Alice")
	u.PasswordHash = "hashed"
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByUsername(ctx, username)
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ID = %v, want %v", got.ID, u.ID)
	}
}
