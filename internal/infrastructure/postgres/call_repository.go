package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/domain/call"
)

type CallRepository struct{ db *sql.DB }

func NewCallRepository(db *sql.DB) call.Repository { return &CallRepository{db: db} }

const callCols = `id, conversation_id, initiator_id, status, media_node_id, started_at, ended_at`

func scanCall(row interface{ Scan(...any) error }) (*call.Call, error) {
	var c call.Call
	var status string
	var ended sql.NullTime
	if err := row.Scan(&c.ID, &c.ConversationID, &c.InitiatorID, &status, &c.MediaNodeID, &c.StartedAt, &ended); err != nil {
		return nil, err
	}
	c.Status = call.Status(status)
	if ended.Valid {
		t := ended.Time
		c.EndedAt = &t
	}
	return &c, nil
}

func (r *CallRepository) Create(ctx context.Context, c *call.Call) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calls (id, conversation_id, initiator_id, status, media_node_id, started_at)
		 VALUES ($1, $2, $3, $4::call_status, $5, $6)`,
		c.ID, c.ConversationID, c.InitiatorID, string(c.Status), c.MediaNodeID, c.StartedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_calls_one_live_per_conversation" {
			return call.ErrLiveCallExists
		}
		return fmt.Errorf("insert call: %w", err)
	}
	return nil
}

func (r *CallRepository) GetByID(ctx context.Context, id uuid.UUID) (*call.Call, error) {
	c, err := scanCall(r.db.QueryRowContext(ctx, `SELECT `+callCols+` FROM calls WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, call.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select call: %w", err)
	}
	return c, nil
}

func (r *CallRepository) GetLiveForConversation(ctx context.Context, conversationID uuid.UUID) (*call.Call, error) {
	c, err := scanCall(r.db.QueryRowContext(ctx,
		`SELECT `+callCols+` FROM calls WHERE conversation_id = $1 AND status = 'active'`, conversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, call.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select live call: %w", err)
	}
	return c, nil
}

func (r *CallRepository) MarkEnded(ctx context.Context, id uuid.UUID, endedAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calls SET status = 'ended', ended_at = $2 WHERE id = $1 AND status = 'active'`, id, endedAt)
	if err != nil {
		return false, fmt.Errorf("mark call ended: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *CallRepository) AddParticipant(ctx context.Context, p *call.Participant) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO call_participants (id, call_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4::call_participant_role, $5)`,
		p.ID, p.CallID, p.UserID, string(p.Role), p.JoinedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_call_participants_one_active" {
			return call.ErrAlreadyInCall
		}
		return fmt.Errorf("insert call participant: %w", err)
	}
	return nil
}

func (r *CallRepository) MarkParticipantLeft(ctx context.Context, callID, userID uuid.UUID, leftAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE call_participants SET left_at = $3
		 WHERE call_id = $1 AND user_id = $2 AND left_at IS NULL`, callID, userID, leftAt)
	if err != nil {
		return fmt.Errorf("mark participant left: %w", err)
	}
	return nil
}

func (r *CallRepository) ListParticipants(ctx context.Context, callID uuid.UUID) ([]*call.Participant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, call_id, user_id, role, joined_at, left_at
		 FROM call_participants WHERE call_id = $1 ORDER BY joined_at ASC, id ASC`, callID)
	if err != nil {
		return nil, fmt.Errorf("query call participants: %w", err)
	}
	defer rows.Close()
	var out []*call.Participant
	for rows.Next() {
		var p call.Participant
		var role string
		var left sql.NullTime
		if err := rows.Scan(&p.ID, &p.CallID, &p.UserID, &role, &p.JoinedAt, &left); err != nil {
			return nil, fmt.Errorf("scan call participant: %w", err)
		}
		p.Role = call.Role(role)
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

func (r *CallRepository) ListLive(ctx context.Context) ([]*call.Call, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+callCols+` FROM calls WHERE status = 'active' ORDER BY started_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query live calls: %w", err)
	}
	defer rows.Close()
	var out []*call.Call
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
