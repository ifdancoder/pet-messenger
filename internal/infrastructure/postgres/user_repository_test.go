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

func TestUserRepository_Update(t *testing.T) {
	repo := NewUserRepository(testDB)
	u, err := user.New("update_target_"+randomUsername(), "Original Name")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	u.PasswordHash = "hash"
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := u.SetDisplayName("Updated Name"); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	u.AvatarURL = "/users/" + u.Username + "/avatar"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.DisplayName != "Updated Name" || got.AvatarURL != u.AvatarURL {
		t.Fatalf("got %+v, want DisplayName=Updated Name AvatarURL=%s", got, u.AvatarURL)
	}
}

func TestUserRepository_Update_UsernameCollision(t *testing.T) {
	repo := NewUserRepository(testDB)
	a, _ := user.New("collide_a_"+randomUsername(), "A")
	a.PasswordHash = "hash"
	_ = repo.Create(context.Background(), a)
	b, _ := user.New("collide_b_"+randomUsername(), "B")
	b.PasswordHash = "hash"
	_ = repo.Create(context.Background(), b)

	if err := b.Rename(a.Username); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := repo.Update(context.Background(), b); !errors.Is(err, user.ErrUsernameTaken) {
		t.Fatalf("got %v, want ErrUsernameTaken", err)
	}
}

func TestUserRepository_Update_NotFound(t *testing.T) {
	repo := NewUserRepository(testDB)
	ghost, _ := user.New("ghost_"+randomUsername(), "Ghost")
	ghost.ID = uuid.Must(uuid.NewV7())
	if err := repo.Update(context.Background(), ghost); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
