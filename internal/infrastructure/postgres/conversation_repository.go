// internal/infrastructure/postgres/conversation_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
)

type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) conversation.Repository {
	return &ConversationRepository{db: db}
}

// Create persists the conversation and its founding members in one transaction.
func (r *ConversationRepository) Create(ctx context.Context, c *conversation.Conversation, members []*conversation.Member) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const convQ = `
		INSERT INTO conversations (id, kind, title, avatar_url, created_at)
		VALUES ($1, $2::conversation_kind, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, convQ, c.ID, string(c.Kind), c.Title, c.AvatarURL, c.CreatedAt); err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}

	const memberQ = `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at, muted_until, last_read_message_id)
		VALUES ($1, $2, $3::conversation_role, $4, $5, $6)`
	for _, m := range members {
		if _, err := tx.ExecContext(ctx, memberQ,
			m.ConversationID, m.UserID, string(m.Role), m.JoinedAt, m.MutedUntil, nullableUUID(m.LastReadMessageID),
		); err != nil {
			return fmt.Errorf("insert conversation member: %w", err)
		}
	}

	return tx.Commit()
}

func (r *ConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*conversation.Conversation, error) {
	const q = `SELECT id, kind, title, avatar_url, created_at FROM conversations WHERE id = $1`
	var (
		c    conversation.Conversation
		kind string
	)
	err := r.db.QueryRowContext(ctx, q, id).Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.CreatedAt)
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
		m         conversation.Member
		role      string
		mutedUntil sql.NullTime
		lastRead  uuid.NullUUID
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

func (r *ConversationRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*conversation.Conversation, error) {
	const q = `
		SELECT c.id, c.kind, c.title, c.avatar_url, c.created_at
		FROM conversations c
		JOIN conversation_members m ON m.conversation_id = c.id
		WHERE m.user_id = $1
		ORDER BY c.created_at DESC, c.id DESC`
	rows, err := r.db.QueryContext(ctx, q, userID)
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
		if err := rows.Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.CreatedAt); err != nil {
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
