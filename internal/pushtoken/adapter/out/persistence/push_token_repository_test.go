// internal/pushtoken/adapter/out/persistence/push_token_repository_test.go
package persistence

import (
	"context"
	"database/sql"
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

	domainpush "pet-messenger/internal/pushtoken/domain"
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

func TestPushTokenRepository(t *testing.T) {
	repo := NewPushTokenRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	tk, _ := domainpush.New(u.ID, domainpush.PlatformFCM, "tok-1")
	got, err := repo.Upsert(ctx, tk)
	if err != nil || got.Token != "tok-1" || got.Platform != domainpush.PlatformFCM {
		t.Fatalf("Upsert = %+v, %v", got, err)
	}

	// conflict refreshes last_used_at, does not duplicate
	tk2, _ := domainpush.New(u.ID, domainpush.PlatformFCM, "tok-1")
	if _, err := repo.Upsert(ctx, tk2); err != nil {
		t.Fatalf("Upsert conflict: %v", err)
	}
	list, _ := repo.ListForUser(ctx, u.ID)
	if len(list) != 1 {
		t.Fatalf("ListForUser len = %d, want 1", len(list))
	}

	// DeleteByToken idempotent
	if err := repo.DeleteByToken(ctx, u.ID, "tok-1"); err != nil {
		t.Fatalf("DeleteByToken: %v", err)
	}
	if err := repo.DeleteByToken(ctx, u.ID, "tok-1"); err != nil {
		t.Fatalf("DeleteByToken (missing): %v", err)
	}
	if list, _ := repo.ListForUser(ctx, u.ID); len(list) != 0 {
		t.Fatalf("after delete len = %d", len(list))
	}

	// DeleteToken (owner-agnostic)
	tk3, _ := domainpush.New(u.ID, domainpush.PlatformWeb, "tok-web")
	_, _ = repo.Upsert(ctx, tk3)
	if err := repo.DeleteToken(ctx, "tok-web"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if list, _ := repo.ListForUser(ctx, u.ID); len(list) != 0 {
		t.Fatalf("after DeleteToken len = %d", len(list))
	}
}
