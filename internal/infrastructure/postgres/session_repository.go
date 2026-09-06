// internal/infrastructure/postgres/session_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domainauth "pet-messenger/internal/domain/auth"
)

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) domainauth.Repository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, s *domainauth.Session) error {
	const q = `
		INSERT INTO sessions (id, user_id, refresh_token_hash, device_info, created_at, last_used_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, q, s.ID, s.UserID, s.RefreshTokenHash, s.DeviceInfo, s.CreatedAt, s.LastUsedAt, s.RevokedAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (r *SessionRepository) GetByRefreshTokenHash(ctx context.Context, hash string) (*domainauth.Session, error) {
	const q = `
		SELECT id, user_id, refresh_token_hash, device_info, created_at, last_used_at, revoked_at
		FROM sessions WHERE refresh_token_hash = $1`
	row := r.db.QueryRowContext(ctx, q, hash)

	var s domainauth.Session
	var revokedAt sql.NullTime
	err := row.Scan(&s.ID, &s.UserID, &s.RefreshTokenHash, &s.DeviceInfo, &s.CreatedAt, &s.LastUsedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainauth.ErrNotFound
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
func (r *SessionRepository) RotateSession(ctx context.Context, oldSessionID uuid.UUID, newSession *domainauth.Session) error {
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

func (r *SessionRepository) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = $2 WHERE id = $1`, sessionID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}
