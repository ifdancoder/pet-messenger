// internal/notification/adapter/out/persistence/notification_repository_test.go
package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
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

	domainnotif "pet-messenger/internal/notification/domain"
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

func mkNotif(t *testing.T, userID uuid.UUID, dedup string) *domainnotif.Notification {
	t.Helper()
	n, err := domainnotif.New(userID, domainnotif.KindMessage, json.RawMessage(`{"k":1}`), dedup)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func idsOfNotif(ns []*domainnotif.Notification) []uuid.UUID {
	out := make([]uuid.UUID, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func TestNotificationRepository_CreateAndDedup(t *testing.T) {
	repo := NewNotificationRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	n1 := mkNotif(t, u.ID, "msg:abc")
	if err := repo.Create(ctx, n1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	n2 := mkNotif(t, u.ID, "msg:abc")
	if err := repo.Create(ctx, n2); !errors.Is(err, domainnotif.ErrDuplicate) {
		t.Fatalf("Create dup = %v, want ErrDuplicate", err)
	}
	u2 := createTestUser(t)
	if err := repo.Create(ctx, mkNotif(t, u2.ID, "msg:abc")); err != nil {
		t.Fatalf("Create for other user: %v", err)
	}
	if err := repo.Create(ctx, mkNotif(t, u.ID, "")); err != nil {
		t.Fatalf("Create keyless: %v", err)
	}
	if err := repo.Create(ctx, mkNotif(t, u.ID, "")); err != nil {
		t.Fatalf("Create keyless #2: %v", err)
	}
}

func TestNotificationRepository_ListMarkCount(t *testing.T) {
	repo := NewNotificationRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	var made []*domainnotif.Notification
	for i := 0; i < 3; i++ {
		n := mkNotif(t, u.ID, "")
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("seed #%d: %v", i, err)
		}
		made = append(made, n)
		time.Sleep(2 * time.Millisecond)
	}

	page, err := repo.ListForUser(ctx, u.ID, time.Time{}, uuid.Nil, 10)
	if err != nil || len(page) != 3 || page[0].ID != made[2].ID || page[2].ID != made[0].ID {
		t.Fatalf("ListForUser = %v, %v", idsOfNotif(page), err)
	}
	if c, _ := repo.CountUnread(ctx, u.ID); c != 3 {
		t.Fatalf("unread = %d, want 3", c)
	}

	mid := made[1]
	n, err := repo.MarkReadThrough(ctx, u.ID, mid.CreatedAt, mid.ID)
	if err != nil || n != 2 {
		t.Fatalf("MarkReadThrough = %d, %v, want 2", n, err)
	}
	if c, _ := repo.CountUnread(ctx, u.ID); c != 1 {
		t.Fatalf("unread after = %d, want 1", c)
	}
	n, _ = repo.MarkReadThrough(ctx, u.ID, mid.CreatedAt, mid.ID)
	if n != 0 {
		t.Fatalf("re-mark = %d, want 0", n)
	}
	n, _ = repo.MarkReadThrough(ctx, u.ID, time.Time{}, uuid.Nil)
	if n != 1 {
		t.Fatalf("mark all = %d, want 1", n)
	}

	p1, _ := repo.ListForUser(ctx, u.ID, time.Time{}, uuid.Nil, 2)
	if len(p1) != 2 {
		t.Fatalf("p1 len = %d", len(p1))
	}
	last := p1[len(p1)-1]
	p2, _ := repo.ListForUser(ctx, u.ID, last.CreatedAt, last.ID, 2)
	if len(p2) != 1 || p2[0].ID != made[0].ID {
		t.Fatalf("p2 = %v", idsOfNotif(p2))
	}
}
