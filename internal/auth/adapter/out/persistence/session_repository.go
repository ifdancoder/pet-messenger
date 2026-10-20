// internal/auth/adapter/out/persistence/session_repository.go

// Package persistence is the auth context's secondary ("driven") Postgres
// adapter.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/auth/application/port/out"
	"pet-messenger/internal/auth/domain"
)

// SessionRepository implements out.SessionStore.
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository constructs the port directly: nothing else in the
// repo needs the concrete type.
func NewSessionRepository(db *sql.DB) out.SessionStore {
	return &SessionRepository{db: db}
}

// Create inserts a new session row.
func (r *SessionRepository) Create(ctx context.Context, s *domain.Session) error {
	const q = `
		INSERT INTO sessions (id, user_id, refresh_token_hash, device_info, created_at, last_used_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, q, s.ID, s.UserID, s.RefreshTokenHash, s.DeviceInfo, s.CreatedAt, s.LastUsedAt, s.RevokedAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetByRefreshTokenHash looks up a session by its refresh-token hash,
// including revoked sessions, returning domain.ErrNotFound if none matches.
func (r *SessionRepository) GetByRefreshTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	const q = `
		SELECT id, user_id, refresh_token_hash, device_info, created_at, last_used_at, revoked_at
		FROM sessions WHERE refresh_token_hash = $1`
	row := r.db.QueryRowContext(ctx, q, hash)

	var s domain.Session
	var revokedAt sql.NullTime
	err := row.Scan(&s.ID, &s.UserID, &s.RefreshTokenHash, &s.DeviceInfo, &s.CreatedAt, &s.LastUsedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	if revokedAt.Valid {
		s.RevokedAt = &revokedAt.Time
	}
	return &s, nil
}

// RotateSession revokes oldSessionID and inserts newSession in a single
// transaction, so a refresh never leaves the store in a half-rotated state.
func (r *SessionRepository) RotateSession(ctx context.Context, oldSessionID uuid.UUID, newSession *domain.Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = $2 WHERE id = $1`, oldSessionID, time.Now().UTC()); err != nil {
		return fmt.Errorf("revoke old session: %w", err)
	}

	const insertQ = `
		INSERT INTO sessions (id, user_id, refresh_token_hash, device_info, created_at, last_used_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := tx.ExecContext(ctx, insertQ, newSession.ID, newSession.UserID, newSession.RefreshTokenHash, newSession.DeviceInfo, newSession.CreatedAt, newSession.LastUsedAt, newSession.RevokedAt); err != nil {
		return fmt.Errorf("insert new session: %w", err)
	}

	return tx.Commit()
}

// Revoke marks a single session as revoked.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = $2 WHERE id = $1`, sessionID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeAllForUser marks every still-active session belonging to userID as
// revoked.
func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}
