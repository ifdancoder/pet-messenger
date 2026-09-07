// internal/infrastructure/postgres/message_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/message"
)

type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) message.Repository {
	return &MessageRepository{db: db}
}

const messageColumns = `id, conversation_id, sender_id, type, content, reply_to_id, edited_at, deleted_at, created_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessage(s rowScanner) (*message.Message, error) {
	var (
		m         message.Message
		typ       string
		sender    uuid.NullUUID
		replyTo   uuid.NullUUID
		editedAt  sql.NullTime
		deletedAt sql.NullTime
	)
	if err := s.Scan(&m.ID, &m.ConversationID, &sender, &typ, &m.Content, &replyTo, &editedAt, &deletedAt, &m.CreatedAt); err != nil {
		return nil, err
	}
	m.Type = message.Type(typ)
	if sender.Valid {
		id := sender.UUID
		m.SenderID = &id
	}
	if replyTo.Valid {
		id := replyTo.UUID
		m.ReplyToID = &id
	}
	if editedAt.Valid {
		t := editedAt.Time
		m.EditedAt = &t
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		m.DeletedAt = &t
	}
	return &m, nil
}

func (r *MessageRepository) Create(ctx context.Context, m *message.Message) error {
	const q = `
		INSERT INTO messages (id, conversation_id, sender_id, type, content, reply_to_id, edited_at, deleted_at, created_at)
		VALUES ($1, $2, $3, $4::message_type, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, q,
		m.ID, m.ConversationID, nullableUUID(m.SenderID), string(m.Type), m.Content,
		nullableUUID(m.ReplyToID), m.EditedAt, m.DeletedAt, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *MessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*message.Message, error) {
	m, err := scanMessage(r.db.QueryRowContext(ctx, `SELECT `+messageColumns+` FROM messages WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, message.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan message: %w", err)
	}
	return m, nil
}

// ListByConversation returns messages ordered by (created_at, id) ascending.
// after, when non-nil, is a keyset cursor: only rows strictly greater than that
// message's (created_at, id) pair are returned. The cursor is resolved by a
// dedicated query scoped to conversationID, so an unknown, hard-deleted, or
// foreign-conversation id surfaces as message.ErrInvalidCursor rather than
// silently filtering out every row. Soft-deleted messages are included (with
// Content cleared). limit <= 0 returns message.ErrInvalidLimit.
func (r *MessageRepository) ListByConversation(ctx context.Context, conversationID uuid.UUID, after *uuid.UUID, limit int) ([]*message.Message, error) {
	if limit <= 0 {
		return nil, message.ErrInvalidLimit
	}

	var (
		cursorAt time.Time
		cursorID uuid.UUID
	)
	if after != nil {
		err := r.db.QueryRowContext(ctx,
			`SELECT created_at, id FROM messages WHERE id = $1 AND conversation_id = $2`,
			*after, conversationID,
		).Scan(&cursorAt, &cursorID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, message.ErrInvalidCursor
		}
		if err != nil {
			return nil, fmt.Errorf("resolve cursor: %w", err)
		}
	}

	afterAt := sql.NullTime{}
	if after != nil {
		afterAt = sql.NullTime{Time: cursorAt, Valid: true}
	}

	const q = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE conversation_id = $1
		  AND ($2::timestamptz IS NULL OR (created_at, id) > ($2, $3))
		ORDER BY created_at, id
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, conversationID, afterAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var out []*message.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message rows: %w", err)
	}
	return out, nil
}

// Update persists a mutated message (Edit / MarkDeleted).
func (r *MessageRepository) Update(ctx context.Context, m *message.Message) error {
	// When m.DeletedAt == nil this is an edit, not a delete: the extra
	// `deleted_at IS NULL` predicate stops a stale edit from resurrecting a row
	// that was soft-deleted in the meantime. Such an edit then affects 0 rows
	// and surfaces as message.ErrNotFound below. A delete keeps matching the row
	// unconditionally so MarkDeleted stays idempotent.
	q := `UPDATE messages SET content = $2, edited_at = $3, deleted_at = $4 WHERE id = $1`
	if m.DeletedAt == nil {
		q += ` AND deleted_at IS NULL`
	}
	res, err := r.db.ExecContext(ctx, q, m.ID, m.Content, m.EditedAt, m.DeletedAt)
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return message.ErrNotFound
	}
	return nil
}
