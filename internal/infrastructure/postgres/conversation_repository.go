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

// adminTierParam maps a domain tier to an INSERT/UPDATE arg: TierNone becomes
// SQL NULL.
func adminTierParam(t conversation.AdminTier) any {
	if t == conversation.TierNone {
		return nil
	}
	return string(t)
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
		INSERT INTO conversations (id, kind, title, avatar_url, public, created_at, last_message_at, direct_key)
		VALUES ($1, $2::conversation_kind, $3, $4, $5, $6, $7, $8)`
	if _, err := tx.ExecContext(ctx, convQ,
		c.ID, string(c.Kind), c.Title, c.AvatarURL, c.Public, c.CreatedAt, lastMessageAt, directKeyParam,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_conversations_direct_key" {
			return conversation.ErrDirectExists
		}
		return fmt.Errorf("insert conversation: %w", err)
	}

	const memberQ = `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at, muted_until, last_read_message_id, is_creator, admin_tier)
		VALUES ($1, $2, $3::conversation_role, $4, $5, $6, $7, $8::conversation_admin_tier)`
	for _, m := range members {
		if _, err := tx.ExecContext(ctx, memberQ,
			c.ID, m.UserID, string(m.Role), m.JoinedAt, m.MutedUntil, nullableUUID(m.LastReadMessageID),
			m.IsCreator, adminTierParam(m.AdminTier),
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
	const q = `SELECT id, kind, title, avatar_url, public, archived_at, created_at, last_message_at FROM conversations WHERE id = $1`
	var (
		c          conversation.Conversation
		kind       string
		archivedAt sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, q, id).Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	c.Kind = conversation.Kind(kind)
	if archivedAt.Valid {
		t := archivedAt.Time
		c.ArchivedAt = &t
	}
	return &c, nil
}

func (r *ConversationRepository) GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error) {
	const q = `
		SELECT conversation_id, user_id, role, joined_at, muted_until, last_read_message_id, is_creator, admin_tier
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`
	var (
		m          conversation.Member
		role       string
		mutedUntil sql.NullTime
		lastRead   uuid.NullUUID
		adminTier  sql.NullString
	)
	err := r.db.QueryRowContext(ctx, q, conversationID, userID).
		Scan(&m.ConversationID, &m.UserID, &role, &m.JoinedAt, &mutedUntil, &lastRead, &m.IsCreator, &adminTier)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation member: %w", err)
	}
	m.Role = conversation.Role(role)
	if adminTier.Valid {
		m.AdminTier = conversation.AdminTier(adminTier.String)
	} else {
		m.AdminTier = conversation.TierNone
	}
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
		SELECT id, kind, title, avatar_url, public, archived_at, created_at, last_message_at
		FROM conversations
		WHERE kind = 'direct' AND direct_key = $1`
	var (
		c          conversation.Conversation
		kind       string
		archivedAt sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, q, directKey(userA, userB)).
		Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan direct conversation: %w", err)
	}
	c.Kind = conversation.Kind(kind)
	if archivedAt.Valid {
		t := archivedAt.Time
		c.ArchivedAt = &t
	}
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
		SELECT c.id, c.kind, c.title, c.avatar_url, c.public, c.archived_at, c.created_at, c.last_message_at,
		       peer.username, peer.display_name
		FROM conversations c
		JOIN conversation_members m ON m.conversation_id = c.id
		LEFT JOIN LATERAL (
			SELECT u.username, u.display_name
			FROM conversation_members other
			JOIN users u ON u.id = other.user_id
			WHERE other.conversation_id = c.id AND other.user_id <> $1
			LIMIT 1
		) peer ON c.kind = 'direct'
		WHERE m.user_id = $1
		  AND c.archived_at IS NULL
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
			c          conversation.Conversation
			kind       string
			archivedAt sql.NullTime
			username   sql.NullString
			display    sql.NullString
		)
		if err := rows.Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt, &username, &display); err != nil {
			return nil, fmt.Errorf("scan conversation row: %w", err)
		}
		c.Kind = conversation.Kind(kind)
		if username.Valid {
			c.DirectUsername = username.String
		}
		if display.Valid {
			c.DirectDisplayName = display.String
		}
		if archivedAt.Valid {
			t := archivedAt.Time
			c.ArchivedAt = &t
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation rows: %w", err)
	}
	return out, nil
}

// AddMember inserts one membership row; a duplicate (the user is already a
// member) is swallowed so a concurrent double-join converges on one row.
func (r *ConversationRepository) AddMember(ctx context.Context, m *conversation.Member) error {
	const q = `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at, muted_until, last_read_message_id)
		VALUES ($1, $2, $3::conversation_role, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, q,
		m.ConversationID, m.UserID, string(m.Role), m.JoinedAt, m.MutedUntil, nullableUUID(m.LastReadMessageID),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "conversation_members_pkey" {
			return nil // the (conversation_id, user_id) PK — already a member
		}
		return fmt.Errorf("insert conversation member: %w", err)
	}
	return nil
}

// RemoveMember deletes the user's membership, or returns ErrNotMember.
func (r *ConversationRepository) RemoveMember(ctx context.Context, conversationID, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete conversation member: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return conversation.ErrNotMember
	}
	return nil
}

// SetMemberTier sets (or clears, with TierNone) the target member's admin_tier.
// Role is untouched. ErrNotMember if there is no such membership row.
func (r *ConversationRepository) SetMemberTier(ctx context.Context, conversationID, userID uuid.UUID, tier conversation.AdminTier) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE conversation_members SET admin_tier = $3::conversation_admin_tier
		 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID, adminTierParam(tier),
	)
	if err != nil {
		return fmt.Errorf("update conversation member tier: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return conversation.ErrNotMember
	}
	return nil
}

// ListPublicChannels returns public channels newest-activity-first with a
// (last_message_at, id) keyset. A non-empty query is a case-insensitive title
// substring filter. The idx_conversations_public_channels partial index backs
// the ORDER BY. limit is defensively clamped.
func (r *ConversationRepository) ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*conversation.Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var beforePtr *time.Time
	if !before.IsZero() {
		b := before.UTC()
		beforePtr = &b
	}

	const q = `
		SELECT id, kind, title, avatar_url, public, archived_at, created_at, last_message_at
		FROM conversations
		WHERE public AND kind IN ('channel_group', 'channel_broadcast')
		  AND archived_at IS NULL
		  AND ($1 = '' OR title ILIKE '%' || $1 || '%')
		  AND ($2::timestamptz IS NULL OR (last_message_at, id) < ($2, $3))
		ORDER BY last_message_at DESC, id DESC
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, query, beforePtr, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("query public channels: %w", err)
	}
	defer rows.Close()

	var out []*conversation.Conversation
	for rows.Next() {
		var (
			c          conversation.Conversation
			kind       string
			archivedAt sql.NullTime
		)
		if err := rows.Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt); err != nil {
			return nil, fmt.Errorf("scan public channel row: %w", err)
		}
		c.Kind = conversation.Kind(kind)
		if archivedAt.Valid {
			t := archivedAt.Time
			c.ArchivedAt = &t
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public channel rows: %w", err)
	}
	return out, nil
}

// ListMemberIDs returns just the user ids of a conversation's members, in
// unspecified order. A missing conversation yields an empty slice and no error.
func (r *ConversationRepository) ListMemberIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM conversation_members WHERE conversation_id = $1`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query member ids: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan member id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate member id rows: %w", err)
	}
	return out, nil
}
