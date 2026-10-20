// internal/e2ee/adapter/out/persistence/device_repository.go

// Package persistence is the e2ee context's secondary ("driven") Postgres
// adapter.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// DeviceRepository implements out.DeviceLoader, out.DeviceWriter,
// out.DeviceListerForUser, out.TrustedDeviceLister, out.KeyEnvelopeWriter,
// and out.KeyEnvelopeReader.
type DeviceRepository struct{ db *sql.DB }

// NewDeviceRepository constructs the adapter. It returns the concrete type
// because it satisfies six ports at once; fx bridges each one separately
// (see internal/fx/e2ee.go).
func NewDeviceRepository(db *sql.DB) *DeviceRepository { return &DeviceRepository{db: db} }

// Register persists d, deciding under a row lock whether it becomes the
// account's first (and therefore auto-trusted) device or a pending one
// awaiting approval. On success d.Status is set to reflect that decision.
func (r *DeviceRepository) Register(ctx context.Context, d *domaine2ee.Device) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin register e2ee device: %w", err)
	}
	defer tx.Rollback()
	// Serialize the account's first-device decision. Without this row lock
	// two simultaneous registrations could both observe zero trusted
	// devices.
	var lockedUser uuid.UUID
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, d.UserID).Scan(&lockedUser); err != nil {
		return fmt.Errorf("lock e2ee device owner: %w", err)
	}
	var hasTrusted bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM e2ee_devices WHERE user_id=$1 AND status='trusted' AND revoked_at IS NULL)`, d.UserID).Scan(&hasTrusted); err != nil {
		return fmt.Errorf("check trusted e2ee device: %w", err)
	}
	if hasTrusted {
		d.Status = domaine2ee.StatusPending
	} else {
		d.Status = domaine2ee.StatusTrusted
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO e2ee_devices (id,user_id,public_key,signing_public_key,fingerprint,label,status,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, d.ID, d.UserID, d.PublicKey, d.SigningPublicKey, d.Fingerprint, d.Label, d.Status, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert e2ee device: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit e2ee device: %w", err)
	}
	return nil
}

// Get loads a device by id, or domain.ErrNotFound if none exists.
func (r *DeviceRepository) Get(ctx context.Context, id uuid.UUID) (*domaine2ee.Device, error) {
	var d domaine2ee.Device
	var status string
	var approvedBy uuid.NullUUID
	var approvedAt, revokedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id,user_id,public_key,signing_public_key,fingerprint,label,status,approved_by,created_at,approved_at,revoked_at FROM e2ee_devices WHERE id=$1`, id).Scan(&d.ID, &d.UserID, &d.PublicKey, &d.SigningPublicKey, &d.Fingerprint, &d.Label, &status, &approvedBy, &d.CreatedAt, &approvedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domaine2ee.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get e2ee device: %w", err)
	}
	d.Status = domaine2ee.Status(status)
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

// ListForUser returns every non-revoked device belonging to userID.
func (r *DeviceRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domaine2ee.Device, error) {
	return r.list(ctx, `SELECT id,user_id,public_key,signing_public_key,fingerprint,label,status,created_at FROM e2ee_devices WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at`, userID)
}

// ListTrustedForConversation returns every trusted device belonging to any
// member of conversationID.
func (r *DeviceRepository) ListTrustedForConversation(ctx context.Context, conversationID uuid.UUID) ([]*domaine2ee.Device, error) {
	return r.list(ctx, `SELECT d.id,d.user_id,d.public_key,d.signing_public_key,d.fingerprint,d.label,d.status,d.created_at FROM e2ee_devices d JOIN conversation_members m ON m.user_id=d.user_id WHERE m.conversation_id=$1 AND d.status='trusted' AND d.revoked_at IS NULL ORDER BY d.user_id,d.created_at`, conversationID)
}

func (r *DeviceRepository) list(ctx context.Context, query string, id uuid.UUID) ([]*domaine2ee.Device, error) {
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domaine2ee.Device
	for rows.Next() {
		d := new(domaine2ee.Device)
		var s string
		if err := rows.Scan(&d.ID, &d.UserID, &d.PublicKey, &d.SigningPublicKey, &d.Fingerprint, &d.Label, &s, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Status = domaine2ee.Status(s)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Approve trusts targetID on approverID's vouching. It returns
// domain.ErrNotTrusted if approverID is not itself trusted, or
// domain.ErrNotFound if targetID is not a pending device owned by the same
// user as approverID.
func (r *DeviceRepository) Approve(ctx context.Context, approverID, targetID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var u uuid.UUID
	var s string
	if err = tx.QueryRowContext(ctx, `SELECT user_id,status FROM e2ee_devices WHERE id=$1 FOR UPDATE`, approverID).Scan(&u, &s); err != nil {
		return domaine2ee.ErrNotTrusted
	}
	if s != "trusted" {
		return domaine2ee.ErrNotTrusted
	}
	var targetUser uuid.UUID
	if err = tx.QueryRowContext(ctx, `SELECT user_id FROM e2ee_devices WHERE id=$1 AND status='pending' FOR UPDATE`, targetID).Scan(&targetUser); err != nil {
		return domaine2ee.ErrNotFound
	}
	if targetUser != u {
		return domaine2ee.ErrNotTrusted
	}
	_, err = tx.ExecContext(ctx, `UPDATE e2ee_devices SET status='trusted',approved_by=$2,approved_at=$3 WHERE id=$1`, targetID, approverID, time.Now().UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PutKeyEnvelopes persists a batch of key envelopes, upserting on the
// (owner, recipient, scope) unique constraint so a resubmission replaces
// the previous ciphertext rather than erroring.
func (r *DeviceRepository) PutKeyEnvelopes(ctx context.Context, envelopes []*domaine2ee.KeyEnvelope) error {
	if len(envelopes) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin put e2ee key envelopes: %w", err)
	}
	defer tx.Rollback()
	for _, envelope := range envelopes {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO e2ee_key_envelopes (id,owner_device_id,recipient_device_id,scope,ciphertext,created_at)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (owner_device_id,recipient_device_id,scope)
			DO UPDATE SET ciphertext=EXCLUDED.ciphertext,created_at=EXCLUDED.created_at`,
			uuid.New(), envelope.OwnerDeviceID, envelope.RecipientDeviceID, envelope.Scope, envelope.Ciphertext, envelope.CreatedAt)
		if err != nil {
			return fmt.Errorf("put e2ee key envelope: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit e2ee key envelopes: %w", err)
	}
	return nil
}

// GetKeyEnvelope returns the most recent key envelope for recipientID in
// scope, or domain.ErrNotFound if none exists.
func (r *DeviceRepository) GetKeyEnvelope(ctx context.Context, recipientID uuid.UUID, scope string) (*domaine2ee.KeyEnvelope, error) {
	var envelope domaine2ee.KeyEnvelope
	err := r.db.QueryRowContext(ctx, `
		SELECT owner_device_id,recipient_device_id,scope,ciphertext,created_at
		FROM e2ee_key_envelopes
		WHERE recipient_device_id=$1 AND scope=$2
		ORDER BY created_at DESC LIMIT 1`, recipientID, scope).
		Scan(&envelope.OwnerDeviceID, &envelope.RecipientDeviceID, &envelope.Scope, &envelope.Ciphertext, &envelope.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domaine2ee.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get e2ee key envelope: %w", err)
	}
	return &envelope, nil
}
