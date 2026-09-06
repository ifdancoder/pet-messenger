// internal/database/database_test.go
package database

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(
		ctx,
		"postgres:latest",
		postgres.WithDatabase("database"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
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

	code := m.Run()

	testDB.Close()
	if err := container.Terminate(ctx); err != nil {
		log.Fatalf("terminate container: %v", err)
	}
	os.Exit(code)
}

func TestHealth(t *testing.T) {
	srv := New(testDB)

	stats := srv.Health()

	if stats["status"] != "up" {
		t.Fatalf("expected status to be up, got %s", stats["status"])
	}
	if _, ok := stats["error"]; ok {
		t.Fatalf("expected error not to be present")
	}
	if stats["message"] != "It's healthy" {
		t.Fatalf("expected message to be 'It's healthy', got %s", stats["message"])
	}
}
