// internal/auth/adapter/out/persistence/session_repository_test.go
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

	"pet-messenger/internal/auth/domain"
	domainuser "pet-messenger/internal/user/domain"
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

	sess := domain.New(u.ID, "hash-create-1", "device-a")
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
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_RotateSession(t *testing.T) {
	repo := NewSessionRepository(testDB)
	u := createTestUser(t)
	ctx := context.Background()

	old := domain.New(u.ID, "hash-old", "device-a")
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newSess := domain.New(u.ID, "hash-new", "device-a")
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

	s1 := domain.New(u.ID, "hash-multi-a", "device-a")
	s2 := domain.New(u.ID, "hash-multi-b", "device-b")
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
