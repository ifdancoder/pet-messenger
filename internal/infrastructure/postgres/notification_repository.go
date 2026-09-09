package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/domain/notification"
)

type NotificationRepository struct{ db *sql.DB }

func NewNotificationRepository(db *sql.DB) notification.Repository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, n *notification.Notification) error {
	var dedup any
	if n.DedupKey != "" {
		dedup = n.DedupKey
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, kind, payload, dedup_key, created_at)
		 VALUES ($1, $2, $3::notification_kind, $4, $5, $6)`,
		n.ID, n.UserID, string(n.Kind), []byte(n.Payload), dedup, n.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_notifications_user_dedup" {
			return notification.ErrDuplicate
		}
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func (r *NotificationRepository) ListForUser(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*notification.Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var beforePtr *time.Time
	if !before.IsZero() {
		b := before.UTC()
		beforePtr = &b
	}
	const q = `
		SELECT id, user_id, kind, payload, dedup_key, created_at, read_at
		FROM notifications
		WHERE user_id = $1
		  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, userID, beforePtr, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()
	var out []*notification.Notification
	for rows.Next() {
		var (
			n       notification.Notification
			kind    string
			payload []byte
			dedup   sql.NullString
			readAt  sql.NullTime
		)
		if err := rows.Scan(&n.ID, &n.UserID, &kind, &payload, &dedup, &n.CreatedAt, &readAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Kind = notification.Kind(kind)
		n.Payload = payload
		n.DedupKey = dedup.String
		if readAt.Valid {
			t := readAt.Time
			n.ReadAt = &t
		}
		out = append(out, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification rows: %w", err)
	}
	return out, nil
}

func (r *NotificationRepository) MarkReadThrough(ctx context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
	var throughPtr *time.Time
	if !throughCreatedAt.IsZero() {
		t := throughCreatedAt.UTC()
		throughPtr = &t
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE user_id = $1 AND read_at IS NULL
		   AND ($2::timestamptz IS NULL OR (created_at, id) <= ($2, $3))`,
		userID, throughPtr, throughID,
	)
	if err != nil {
		return 0, fmt.Errorf("mark notifications read: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var c int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&c)
	if err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return c, nil
}
