package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/pushtoken"
)

type PushTokenRepository struct{ db *sql.DB }

func NewPushTokenRepository(db *sql.DB) pushtoken.Repository {
	return &PushTokenRepository{db: db}
}

func (r *PushTokenRepository) Upsert(ctx context.Context, t *pushtoken.PushToken) (*pushtoken.PushToken, error) {
	const q = `
		INSERT INTO push_tokens (id, user_id, platform, token, created_at, last_used_at)
		VALUES ($1, $2, $3::push_platform, $4, $5, $6)
		ON CONFLICT (user_id, token) DO UPDATE SET last_used_at = EXCLUDED.last_used_at
		RETURNING id, user_id, platform, token, created_at, last_used_at`
	var (
		out      pushtoken.PushToken
		platform string
	)
	err := r.db.QueryRowContext(ctx, q,
		t.ID, t.UserID, string(t.Platform), t.Token, t.CreatedAt, t.LastUsedAt,
	).Scan(&out.ID, &out.UserID, &platform, &out.Token, &out.CreatedAt, &out.LastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert push token: %w", err)
	}
	out.Platform = pushtoken.Platform(platform)
	return &out, nil
}

func (r *PushTokenRepository) DeleteByToken(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM push_tokens WHERE user_id = $1 AND token = $2`, userID, token)
	if err != nil {
		return fmt.Errorf("delete push token: %w", err)
	}
	return nil
}

func (r *PushTokenRepository) DeleteToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM push_tokens WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete push token: %w", err)
	}
	return nil
}

func (r *PushTokenRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*pushtoken.PushToken, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, platform, token, created_at, last_used_at
		 FROM push_tokens WHERE user_id = $1 ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query push tokens: %w", err)
	}
	defer rows.Close()
	var out []*pushtoken.PushToken
	for rows.Next() {
		var (
			t        pushtoken.PushToken
			platform string
		)
		if err := rows.Scan(&t.ID, &t.UserID, &platform, &t.Token, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan push token: %w", err)
		}
		t.Platform = pushtoken.Platform(platform)
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate push token rows: %w", err)
	}
	return out, nil
}
