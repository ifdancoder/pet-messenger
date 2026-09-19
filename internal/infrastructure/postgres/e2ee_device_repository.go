package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/e2ee"
)

type E2EEDeviceRepository struct{ db *sql.DB }

func NewE2EEDeviceRepository(db *sql.DB) e2ee.Repository { return &E2EEDeviceRepository{db: db} }

func (r *E2EEDeviceRepository) Register(ctx context.Context, d *e2ee.Device) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin register e2ee device: %w", err)
	}
	defer tx.Rollback()
	// Serialize the account's first-device decision. Without this row lock two
	// simultaneous registrations could both observe zero trusted devices.
	var lockedUser uuid.UUID
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, d.UserID).Scan(&lockedUser); err != nil {
		return fmt.Errorf("lock e2ee device owner: %w", err)
	}
	var hasTrusted bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM e2ee_devices WHERE user_id=$1 AND status='trusted' AND revoked_at IS NULL)`, d.UserID).Scan(&hasTrusted); err != nil {
		return fmt.Errorf("check trusted e2ee device: %w", err)
	}
	if hasTrusted {
		d.Status = e2ee.StatusPending
	} else {
		d.Status = e2ee.StatusTrusted
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO e2ee_devices (id,user_id,public_key,fingerprint,label,status,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, d.ID, d.UserID, d.PublicKey, d.Fingerprint, d.Label, d.Status, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert e2ee device: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit e2ee device: %w", err)
	}
	return nil
}

func (r *E2EEDeviceRepository) Get(ctx context.Context, id uuid.UUID) (*e2ee.Device, error) {
	var d e2ee.Device
	var status string
	var approvedBy uuid.NullUUID
	var approvedAt, revokedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id,user_id,public_key,fingerprint,label,status,approved_by,created_at,approved_at,revoked_at FROM e2ee_devices WHERE id=$1`, id).Scan(&d.ID, &d.UserID, &d.PublicKey, &d.Fingerprint, &d.Label, &status, &approvedBy, &d.CreatedAt, &approvedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, e2ee.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get e2ee device: %w", err)
	}
	d.Status = e2ee.Status(status)
	if approvedBy.Valid {
		v := approvedBy.UUID
		d.ApprovedBy = &v
	}
	if approvedAt.Valid {
		v := approvedAt.Time
		d.ApprovedAt = &v
	}
	if revokedAt.Valid {
		v := revokedAt.Time
		d.RevokedAt = &v
	}
	return &d, nil
}

func (r *E2EEDeviceRepository) ListTrusted(ctx context.Context, userID uuid.UUID) ([]*e2ee.Device, error) {
	return r.list(ctx, `SELECT id,user_id,public_key,fingerprint,label,status,created_at FROM e2ee_devices WHERE user_id=$1 AND status='trusted' AND revoked_at IS NULL ORDER BY created_at`, userID)
}

func (r *E2EEDeviceRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*e2ee.Device, error) {
	return r.list(ctx, `SELECT id,user_id,public_key,fingerprint,label,status,created_at FROM e2ee_devices WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at`, userID)
}

func (r *E2EEDeviceRepository) ListTrustedForConversation(ctx context.Context, conversationID uuid.UUID) ([]*e2ee.Device, error) {
	return r.list(ctx, `SELECT d.id,d.user_id,d.public_key,d.fingerprint,d.label,d.status,d.created_at FROM e2ee_devices d JOIN conversation_members m ON m.user_id=d.user_id WHERE m.conversation_id=$1 AND d.status='trusted' AND d.revoked_at IS NULL ORDER BY d.user_id,d.created_at`, conversationID)
}

func (r *E2EEDeviceRepository) list(ctx context.Context, query string, id uuid.UUID) ([]*e2ee.Device, error) {
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*e2ee.Device
	for rows.Next() {
		d := new(e2ee.Device)
		var s string
		if err := rows.Scan(&d.ID, &d.UserID, &d.PublicKey, &d.Fingerprint, &d.Label, &s, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Status = e2ee.Status(s)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *E2EEDeviceRepository) Approve(ctx context.Context, approverID, targetID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var u uuid.UUID
	var s string
	if err = tx.QueryRowContext(ctx, `SELECT user_id,status FROM e2ee_devices WHERE id=$1 FOR UPDATE`, approverID).Scan(&u, &s); err != nil {
		return e2ee.ErrNotTrusted
	}
	if s != "trusted" {
		return e2ee.ErrNotTrusted
	}
	var targetUser uuid.UUID
	if err = tx.QueryRowContext(ctx, `SELECT user_id FROM e2ee_devices WHERE id=$1 AND status='pending' FOR UPDATE`, targetID).Scan(&targetUser); err != nil {
		return e2ee.ErrNotFound
	}
	if targetUser != u {
		return e2ee.ErrNotTrusted
	}
	_, err = tx.ExecContext(ctx, `UPDATE e2ee_devices SET status='trusted',approved_by=$2,approved_at=$3 WHERE id=$1`, targetID, approverID, time.Now().UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}
