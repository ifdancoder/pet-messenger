// internal/user/adapter/out/persistence/user_repository_test.go
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"pet-messenger/internal/user/domain"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:latest",
		tcpostgres.WithDatabase("database"),
		tcpostgres.WithUsername("user"),
		tcpostgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	driver, err := migratepg.WithInstance(testDB, &migratepg.Config{})
	if err != nil {
		log.Fatalf("init migrate driver: %v", err)
	}
	mig, err := migrate.NewWithDatabaseInstance("file://../../../../../db/migrations", "pgx", driver)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	code := m.Run()

	testDB.Close()
	if err := container.Terminate(ctx); err != nil {
		log.Fatalf("terminate container: %v", err)
	}

	os.Exit(code)
}

func randomUsername() string {
	return "u_" + uuid.NewString()[:8]
}

func createTestUser(t *testing.T, repo *UserRepository) *domain.User {
	t.Helper()
	u, err := domain.New(randomUsername(), "Test User")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return u
}

func TestUserRepository_CreateAndGetByID(t *testing.T) {
	repo := NewUserRepository(testDB)
	u := createTestUser(t, repo)

	got, err := repo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Username != u.Username {
		t.Fatalf("Username = %q, want %q", got.Username, u.Username)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	repo := NewUserRepository(testDB)
	_, err := repo.GetByID(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_GetByUsername(t *testing.T) {
	repo := NewUserRepository(testDB)
	u := createTestUser(t, repo)

	got, err := repo.GetByUsername(context.Background(), u.Username)
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("ID = %v, want %v", got.ID, u.ID)
	}
}

func TestUserRepository_Create_DuplicateUsername(t *testing.T) {
	repo := NewUserRepository(testDB)
	u := createTestUser(t, repo)

	dup, err := domain.New(u.Username, "Someone Else")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	dup.PasswordHash = "hashed"
	if err := repo.Create(context.Background(), dup); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestUserRepository_Update(t *testing.T) {
	repo := NewUserRepository(testDB)
	u := createTestUser(t, repo)

	if err := u.SetDisplayName("Updated Name"); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.DisplayName != "Updated Name" {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, "Updated Name")
	}
}

func TestUserRepository_Update_NotFound(t *testing.T) {
	repo := NewUserRepository(testDB)
	u, err := domain.New(randomUsername(), "Ghost")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	if err := repo.Update(context.Background(), u); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserRepository_Update_UsernameTaken(t *testing.T) {
	repo := NewUserRepository(testDB)
	alice := createTestUser(t, repo)
	bob := createTestUser(t, repo)

	bob.Username = alice.Username
	if err := repo.Update(context.Background(), bob); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestUserRepository_GetProfilesByIDs(t *testing.T) {
	repo := NewUserRepository(testDB)
	a := createTestUser(t, repo)
	b := createTestUser(t, repo)

	profiles, err := repo.GetProfilesByIDs(context.Background(), []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("GetProfilesByIDs: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("len(profiles) = %d, want 2", len(profiles))
	}
	byID := make(map[uuid.UUID]*domain.Profile, 2)
	for _, p := range profiles {
		byID[p.ID] = p
	}
	if byID[a.ID] == nil || byID[a.ID].Username != a.Username {
		t.Errorf("profile for a missing or wrong: %+v", byID[a.ID])
	}
	if byID[b.ID] == nil || byID[b.ID].Username != b.Username {
		t.Errorf("profile for b missing or wrong: %+v", byID[b.ID])
	}
}

func TestUserRepository_GetProfilesByIDs_PartialMiss(t *testing.T) {
	repo := NewUserRepository(testDB)
	a := createTestUser(t, repo)

	profiles, err := repo.GetProfilesByIDs(context.Background(), []uuid.UUID{a.ID, uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatalf("GetProfilesByIDs: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != a.ID {
		t.Fatalf("profiles = %+v, want exactly one entry for a", profiles)
	}
}

func TestUserRepository_GetProfilesByIDs_Empty(t *testing.T) {
	repo := NewUserRepository(testDB)
	profiles, err := repo.GetProfilesByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetProfilesByIDs: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("len(profiles) = %d, want 0", len(profiles))
	}
}
