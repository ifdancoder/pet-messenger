// Package persistence is the call context's secondary ("driven") adapter
// onto Postgres: one CallRepository satisfying all 4 of this context's
// split persistence ports.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	domain "pet-messenger/internal/call/domain"
)

// CallRepository implements out.CallWriter, out.CallLoader,
// out.ParticipantWriter, and out.ParticipantLister against Postgres.
type CallRepository struct{ db *sql.DB }

// NewCallRepository wires a CallRepository against db.
func NewCallRepository(db *sql.DB) *CallRepository { return &CallRepository{db: db} }

const callCols = `id, conversation_id, initiator_id, status, media_node_id, started_at, ended_at`

func scanCall(row interface{ Scan(...any) error }) (*domain.Call, error) {
	var c domain.Call
	var status string
	var ended sql.NullTime
	if err := row.Scan(&c.ID, &c.ConversationID, &c.InitiatorID, &status, &c.MediaNodeID, &c.StartedAt, &ended); err != nil {
		return nil, err
	}
	c.Status = domain.Status(status)
	if ended.Valid {
		t := ended.Time
		c.EndedAt = &t
	}
	return &c, nil
}

// Create inserts c. A second live call for the same conversation violates
// idx_calls_one_live_per_conversation; the caller treats the resulting
// domain.ErrLiveCallExists as "lost the race" and re-reads.
func (r *CallRepository) Create(ctx context.Context, c *domain.Call) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calls (id, conversation_id, initiator_id, status, media_node_id, started_at)
		 VALUES ($1, $2, $3, $4::call_status, $5, $6)`,
		c.ID, c.ConversationID, c.InitiatorID, string(c.Status), c.MediaNodeID, c.StartedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_calls_one_live_per_conversation" {
			return domain.ErrLiveCallExists
		}
		return fmt.Errorf("insert call: %w", err)
	}
	return nil
}

// GetByID returns the call with id, or domain.ErrNotFound.
func (r *CallRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Call, error) {
	c, err := scanCall(r.db.QueryRowContext(ctx, `SELECT `+callCols+` FROM calls WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select call: %w", err)
	}
	return c, nil
}

// GetLiveForConversation returns conversationID's single active call, or
// domain.ErrNotFound when there is none.
func (r *CallRepository) GetLiveForConversation(ctx context.Context, conversationID uuid.UUID) (*domain.Call, error) {
	c, err := scanCall(r.db.QueryRowContext(ctx,
		`SELECT `+callCols+` FROM calls WHERE conversation_id = $1 AND status = 'active'`, conversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select live call: %w", err)
	}
	return c, nil
}

// MarkEnded sets status='ended', ended_at=endedAt for a live call,
// reporting whether a row actually transitioned.
func (r *CallRepository) MarkEnded(ctx context.Context, id uuid.UUID, endedAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calls SET status = 'ended', ended_at = $2 WHERE id = $1 AND status = 'active'`, id, endedAt)
	if err != nil {
		return false, fmt.Errorf("mark call ended: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddParticipant inserts p. An existing active (call_id, user_id) row
// returns domain.ErrAlreadyInCall and inserts nothing.
func (r *CallRepository) AddParticipant(ctx context.Context, p *domain.Participant) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO call_participants (id, call_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4::call_participant_role, $5)`,
		p.ID, p.CallID, p.UserID, string(p.Role), p.JoinedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_call_participants_one_active" {
			return domain.ErrAlreadyInCall
		}
		return fmt.Errorf("insert call participant: %w", err)
	}
	return nil
}

// MarkParticipantLeft stamps left_at on userID's active row for callID. A
// missing active row is not an error (idempotent).
func (r *CallRepository) MarkParticipantLeft(ctx context.Context, callID, userID uuid.UUID, leftAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE call_participants SET left_at = $3
		 WHERE call_id = $1 AND user_id = $2 AND left_at IS NULL`, callID, userID, leftAt)
	if err != nil {
		return fmt.Errorf("mark participant left: %w", err)
	}
	return nil
}

// ListParticipants returns every participant row for callID, oldest
// first.
func (r *CallRepository) ListParticipants(ctx context.Context, callID uuid.UUID) ([]*domain.Participant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, call_id, user_id, role, joined_at, left_at
		 FROM call_participants WHERE call_id = $1 ORDER BY joined_at ASC, id ASC`, callID)
	if err != nil {
		return nil, fmt.Errorf("query call participants: %w", err)
	}
	defer rows.Close()
	var out []*domain.Participant
	for rows.Next() {
		var p domain.Participant
		var role string
		var left sql.NullTime
		if err := rows.Scan(&p.ID, &p.CallID, &p.UserID, &role, &p.JoinedAt, &left); err != nil {
			return nil, fmt.Errorf("scan call participant: %w", err)
		}
		p.Role = domain.Role(role)
		if left.Valid {
			t := left.Time
			p.LeftAt = &t
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate call participants: %w", err)
	}
	return out, nil
}

// ListLive returns every currently-active call, oldest first.
func (r *CallRepository) ListLive(ctx context.Context) ([]*domain.Call, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+callCols+` FROM calls WHERE status = 'active' ORDER BY started_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query live calls: %w", err)
	}
	defer rows.Close()
	var out []*domain.Call
	for rows.Next() {
		c, err := scanCall(rows)
		if err != nil {
			return nil, fmt.Errorf("scan live call: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate live calls: %w", err)
	}
	return out, nil
}
