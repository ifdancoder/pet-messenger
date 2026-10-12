package main

import (
	"context"
	"database/sql"
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
)

func newMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:latest",
		tcpostgres.WithDatabase("database"),
		tcpostgres.WithUsername("user"),
		tcpostgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		t.Fatalf("init migrate driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../db/migrations", "pgx", driver)
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func TestRebuild_PopulatesInboxFromWriteTables(t *testing.T) {
	db := newMigratedTestDB(t)
	ctx := context.Background()

	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	for _, u := range []struct {
		id   uuid.UUID
		name string
	}{{a, "amy"}, {b, "ben"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $2, 'x', now())`, u.id, u.name); err != nil {
			t.Fatalf("insert user %s: %v", u.name, err)
		}
	}
	convID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO conversations (id, kind, title, avatar_url, public, created_at, last_message_at, direct_key) VALUES ($1, 'direct', '', '', false, $2, $2, $3)`, convID, now, a.String()+":"+b.String()); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	for _, u := range []uuid.UUID{a, b} {
		if _, err := db.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, joined_at) VALUES ($1, $2, 'member', $3)`, convID, u, now); err != nil {
			t.Fatalf("insert member %s: %v", u, err)
		}
	}

	n, err := rebuild(db)
	if err != nil {
		t.Fatalf("rebuild() error = %v", err)
	}
	if n != 2 {
		t.Fatalf("rebuild() rows = %d, want 2", n)
	}

	var peer uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT direct_peer_user_id FROM conversation_inbox WHERE user_id = $1 AND conversation_id = $2`, a, convID).Scan(&peer); err != nil {
		t.Fatalf("query rebuilt row: %v", err)
	}
	if peer != b {
		t.Fatalf("direct_peer_user_id = %s, want %s", peer, b)
	}
}

func TestRebuild_ClearsStaleRows(t *testing.T) {
	db := newMigratedTestDB(t)
	ctx := context.Background()

	owner := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, 'owner', 'owner', 'x', now())`, owner); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	convID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO conversations (id, kind, title, avatar_url, public, created_at, last_message_at) VALUES ($1, 'group_chat', 'g', '', false, $2, $2)`, convID, now); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, joined_at) VALUES ($1, $2, 'member', $3)`, convID, owner, now); err != nil {
		t.Fatalf("insert member: %v", err)
	}

	if _, err := rebuild(db); err != nil {
		t.Fatalf("first rebuild() error = %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM conversation_inbox WHERE user_id = $1 AND conversation_id = $2`, owner, convID).Scan(&count); err != nil {
		t.Fatalf("count after first rebuild: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows after first rebuild = %d, want 1", count)
	}

	// Simulate the membership ending without the projection having been
	// told (a dropped/lost event) — a correct rebuild must clear the now-
	// stale inbox row, because it starts from TRUNCATE, not an upsert.
	if _, err := db.ExecContext(ctx, `DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`, convID, owner); err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if _, err := rebuild(db); err != nil {
		t.Fatalf("second rebuild() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM conversation_inbox WHERE user_id = $1 AND conversation_id = $2`, owner, convID).Scan(&count); err != nil {
		t.Fatalf("count after second rebuild: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows after second rebuild = %d, want 0 (stale row must be cleared)", count)
	}
}
