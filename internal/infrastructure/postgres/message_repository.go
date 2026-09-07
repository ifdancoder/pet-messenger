// internal/infrastructure/postgres/message_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/message"
)

type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) message.Repository {
	return &MessageRepository{db: db}
}

const messageColumns = `id, conversation_id, seq, sender_id, type, content, reply_to_id, edited_at, deleted_at, created_at`

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
	if err := s.Scan(&m.ID, &m.ConversationID, &m.Seq, &sender, &typ, &m.Content, &replyTo, &editedAt, &deletedAt, &m.CreatedAt); err != nil {
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

// Create allocates the message's per-conversation sequence and inserts the row
// in one transaction. The UPDATE ... RETURNING on the parent conversation takes
// a row lock held until COMMIT, so concurrent sends to the same conversation are
// serialized and seq is gap-free and in commit order. On success m.Seq is set.
//
// The row-lock serialization relies on READ COMMITTED semantics (a blocked
// UPDATE re-evaluates against the committed row), which this transaction pins
// explicitly; under a higher isolation level the same interleaving would raise a
// 40001 serialization error instead.
//
// Create locks the conversations row first, then the FK targets (users,
// messages); any future usecase sharing this transaction must keep that lock
// order to avoid deadlock. The spec §4 NATS publish happens after Create returns,
// never inside this transaction (it holds the conversation row lock).
func (r *MessageRepository) Create(ctx context.Context, m *message.Message) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var seq int64
	err = tx.QueryRowContext(ctx,
		`UPDATE conversations SET last_seq = last_seq + 1 WHERE id = $1 RETURNING last_seq`,
		m.ConversationID,
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("allocate seq: %w", message.ErrConversationNotFound)
	}
	if err != nil {
		return fmt.Errorf("allocate seq: %w", err)
	}

	const insertQ = `
		INSERT INTO messages (id, conversation_id, seq, sender_id, type, content, reply_to_id, edited_at, deleted_at, created_at)
		VALUES ($1, $2, $3, $4, $5::message_type, $6, $7, $8, $9, $10)`
	if _, err := tx.ExecContext(ctx, insertQ,
		m.ID, m.ConversationID, seq, nullableUUID(m.SenderID), string(m.Type), m.Content,
		nullableUUID(m.ReplyToID), m.EditedAt, m.DeletedAt, m.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	m.Seq = seq
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

// ListByConversation returns the conversation's messages ordered by seq
// ascending, starting after afterSeq (afterSeq <= 0 means from the first
// message). seq is a per-conversation monotonic sequence allocated under a row
// lock at commit time, so the page is gap-free and in commit order — a client
// that passes the highest seq of the contiguous prefix it holds never misses a
// message (spec §4). Because seq is gap-free, a client that receives a seq more
// than one above its last contiguous seq knows it has a gap and must re-fetch
// from the contiguous watermark. Soft-deleted messages are returned, with
// Content cleared. limit <= 0 returns message.ErrInvalidLimit.
func (r *MessageRepository) ListByConversation(ctx context.Context, conversationID uuid.UUID, afterSeq int64, limit int) ([]*message.Message, error) {
	if limit <= 0 {
		return nil, message.ErrInvalidLimit
	}
	if afterSeq < 0 {
		afterSeq = 0
	}

	const q = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE conversation_id = $1 AND seq > $2
		ORDER BY seq
		LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, conversationID, afterSeq, limit)
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

// ListBeforeSeq returns the conversation's messages ordered by seq DESCENDING
// (newest first), those with seq < beforeSeq. beforeSeq <= 0 means "from the
// newest message". This is the history-scroll read (open a conversation, then
// page upward); the forward catch-up read is ListByConversation. It uses the
// (conversation_id, seq) index via a backward scan. Soft-deleted messages are
// returned, with Content cleared. limit <= 0 returns message.ErrInvalidLimit.
func (r *MessageRepository) ListBeforeSeq(ctx context.Context, conversationID uuid.UUID, beforeSeq int64, limit int) ([]*message.Message, error) {
	if limit <= 0 {
		return nil, message.ErrInvalidLimit
	}

	const q = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE conversation_id = $1 AND ($2 <= 0 OR seq < $2)
		ORDER BY seq DESC
		LIMIT $3`
	rows, err := r.db.QueryContext(ctx, q, conversationID, beforeSeq, limit)
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
