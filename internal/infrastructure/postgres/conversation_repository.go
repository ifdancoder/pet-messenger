// internal/infrastructure/postgres/conversation_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/domain/conversation"
)

type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) conversation.Repository {
	return &ConversationRepository{db: db}
}

// directKey builds the order-independent uniqueness key for a direct
// conversation from its two member ids: the smaller uuid string, a colon, then
// the larger. It is stored in conversations.direct_key and matched by GetDirect.
func directKey(a, b uuid.UUID) string {
	x, y := a.String(), b.String()
	if x > y {
		x, y = y, x
	}
	return x + ":" + y
}

// Create persists the conversation and its founding members in one transaction.
func (r *ConversationRepository) Create(ctx context.Context, c *conversation.Conversation, members []*conversation.Member) error {
	if len(members) == 0 {
		return fmt.Errorf("create conversation: no members")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var directKeyParam any // nil -> SQL NULL for non-direct conversations
	if c.Kind == conversation.KindDirect {
		if len(members) != 2 {
			return fmt.Errorf("create direct conversation: want 2 members, got %d", len(members))
		}
		directKeyParam = directKey(members[0].UserID, members[1].UserID)
	}

	lastMessageAt := c.LastMessageAt
	if lastMessageAt.IsZero() {
		lastMessageAt = c.CreatedAt
	}

	const convQ = `
		INSERT INTO conversations (id, kind, title, avatar_url, created_at, last_message_at, direct_key)
		VALUES ($1, $2::conversation_kind, $3, $4, $5, $6, $7)`
	if _, err := tx.ExecContext(ctx, convQ,
		c.ID, string(c.Kind), c.Title, c.AvatarURL, c.CreatedAt, lastMessageAt, directKeyParam,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_conversations_direct_key" {
			return conversation.ErrDirectExists
		}
		return fmt.Errorf("insert conversation: %w", err)
	}

	const memberQ = `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at, muted_until, last_read_message_id)
		VALUES ($1, $2, $3::conversation_role, $4, $5, $6)`
	for _, m := range members {
		if _, err := tx.ExecContext(ctx, memberQ,
			c.ID, m.UserID, string(m.Role), m.JoinedAt, m.MutedUntil, nullableUUID(m.LastReadMessageID),
		); err != nil {
			return fmt.Errorf("insert conversation member: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *ConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*conversation.Conversation, error) {
	const q = `SELECT id, kind, title, avatar_url, created_at, last_message_at FROM conversations WHERE id = $1`
	var (
		c    conversation.Conversation
		kind string
	)
	err := r.db.QueryRowContext(ctx, q, id).Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	c.Kind = conversation.Kind(kind)
	return &c, nil
}

func (r *ConversationRepository) GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error) {
	const q = `
		SELECT conversation_id, user_id, role, joined_at, muted_until, last_read_message_id
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`
	var (
		m          conversation.Member
		role       string
		mutedUntil sql.NullTime
		lastRead   uuid.NullUUID
	)
	err := r.db.QueryRowContext(ctx, q, conversationID, userID).
		Scan(&m.ConversationID, &m.UserID, &role, &m.JoinedAt, &mutedUntil, &lastRead)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation member: %w", err)
	}
	m.Role = conversation.Role(role)
	if mutedUntil.Valid {
		t := mutedUntil.Time
		m.MutedUntil = &t
	}
	if lastRead.Valid {
		id := lastRead.UUID
		m.LastReadMessageID = &id
	}
	return &m, nil
}

func (r *ConversationRepository) GetDirect(ctx context.Context, userA, userB uuid.UUID) (*conversation.Conversation, error) {
	const q = `
		SELECT id, kind, title, avatar_url, created_at, last_message_at
		FROM conversations
		WHERE kind = 'direct' AND direct_key = $1`
	var (
		c    conversation.Conversation
		kind string
	)
	err := r.db.QueryRowContext(ctx, q, directKey(userA, userB)).
		Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan direct conversation: %w", err)
	}
	c.Kind = conversation.Kind(kind)
	return &c, nil
}

// ListForUser returns the user's conversations newest-activity-first, using a
// keyset on (last_message_at, id). A zero `before` starts from the newest row;
// otherwise only rows strictly older than the (before, beforeID) cursor are
// returned. The idx_conversations_last_message_at index backs the ORDER BY.
// limit is defensively clamped; callers pass an already-validated positive value.
func (r *ConversationRepository) ListForUser(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*conversation.Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var beforePtr *time.Time
	if !before.IsZero() {
		b := before.UTC()
		beforePtr = &b
	}

	const q = `
		SELECT c.id, c.kind, c.title, c.avatar_url, c.created_at, c.last_message_at
		FROM conversations c
		JOIN conversation_members m ON m.conversation_id = c.id
		WHERE m.user_id = $1
		  AND ($2::timestamptz IS NULL OR (c.last_message_at, c.id) < ($2, $3))
		ORDER BY c.last_message_at DESC, c.id DESC
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, userID, beforePtr, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("query conversations for user: %w", err)
	}
	defer rows.Close()

	var out []*conversation.Conversation
	for rows.Next() {
		var (
			c    conversation.Conversation
			kind string
		)
		if err := rows.Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.CreatedAt, &c.LastMessageAt); err != nil {
			return nil, fmt.Errorf("scan conversation row: %w", err)
		}
		c.Kind = conversation.Kind(kind)
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation rows: %w", err)
	}
	return out, nil
}
