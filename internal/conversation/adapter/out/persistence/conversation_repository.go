// internal/conversation/adapter/out/persistence/conversation_repository.go
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/conversation/domain"
)

type ConversationRepository struct {
	db *sql.DB
}

// NewConversationRepository wires the Postgres adapter. The returned type
// is concrete (not hidden behind a port interface) because it satisfies
// several of this context's out ports at once (LoadConversationPort,
// UpdateConversationPort, PublicChannelViewPort, MemberViewPort) plus the
// narrow read-only interfaces other bounded contexts define for
// themselves (message, call, notify, ws) — see internal/fx/conversation.go.
func NewConversationRepository(db *sql.DB) *ConversationRepository {
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
func adminTierParam(t domain.AdminTier) any {
	if t == domain.TierNone {
		return nil
	}
	return string(t)
}

// Create persists the conversation and its founding members in one transaction.
func (r *ConversationRepository) Create(ctx context.Context, c *domain.Conversation, members []*domain.Member) error {
	if len(members) == 0 {
		return fmt.Errorf("create conversation: no members")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var directKeyParam any // nil -> SQL NULL for non-direct conversations
	if c.Kind == domain.KindDirect {
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
			return domain.ErrDirectExists
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

func (r *ConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Conversation, error) {
	const q = `SELECT id, kind, title, avatar_url, public, archived_at, created_at, last_message_at FROM conversations WHERE id = $1`
	var (
		c          domain.Conversation
		kind       string
		archivedAt sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, q, id).Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	c.Kind = domain.Kind(kind)
	if archivedAt.Valid {
		t := archivedAt.Time
		c.ArchivedAt = &t
	}
	return &c, nil
}

func (r *ConversationRepository) GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domain.Member, error) {
	const q = `
		SELECT conversation_id, user_id, role, joined_at, muted_until, last_read_message_id, is_creator, admin_tier
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`
	var (
		m          domain.Member
		role       string
		mutedUntil sql.NullTime
		lastRead   uuid.NullUUID
		adminTier  sql.NullString
	)
	err := r.db.QueryRowContext(ctx, q, conversationID, userID).
		Scan(&m.ConversationID, &m.UserID, &role, &m.JoinedAt, &mutedUntil, &lastRead, &m.IsCreator, &adminTier)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation member: %w", err)
	}
	m.Role = domain.Role(role)
	if adminTier.Valid {
		m.AdminTier = domain.AdminTier(adminTier.String)
	} else {
		m.AdminTier = domain.TierNone
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

func (r *ConversationRepository) GetDirect(ctx context.Context, userA, userB uuid.UUID) (*domain.Conversation, error) {
	const q = `
		SELECT id, kind, title, avatar_url, public, archived_at, created_at, last_message_at
		FROM conversations
		WHERE kind = 'direct' AND direct_key = $1`
	var (
		c          domain.Conversation
		kind       string
		archivedAt sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, q, directKey(userA, userB)).
		Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan direct conversation: %w", err)
	}
	c.Kind = domain.Kind(kind)
	if archivedAt.Valid {
		t := archivedAt.Time
		c.ArchivedAt = &t
	}
	return &c, nil
}

// AddMember inserts one membership row; a duplicate (the user is already a
// member) is swallowed so a concurrent double-join converges on one row.
func (r *ConversationRepository) AddMember(ctx context.Context, m *domain.Member) error {
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

// RemoveMember deletes the user's membership, or returns domain.ErrNotMember.
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
		return domain.ErrNotMember
	}
	return nil
}

// SetMemberTier sets (or clears, with TierNone) the target member's admin_tier.
// Role is untouched. domain.ErrNotMember if there is no such membership row.
func (r *ConversationRepository) SetMemberTier(ctx context.Context, conversationID, userID uuid.UUID, tier domain.AdminTier) error {
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
		return domain.ErrNotMember
	}
	return nil
}

// ListPublicChannels returns public channels newest-activity-first with a
// (last_message_at, id) keyset. A non-empty query is a case-insensitive title
// substring filter. The idx_conversations_public_channels partial index backs
// the ORDER BY. limit is defensively clamped.
func (r *ConversationRepository) ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domain.Conversation, error) {
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

	var out []*domain.Conversation
	for rows.Next() {
		var (
			c          domain.Conversation
			kind       string
			archivedAt sql.NullTime
		)
		if err := rows.Scan(&c.ID, &kind, &c.Title, &c.AvatarURL, &c.Public, &archivedAt, &c.CreatedAt, &c.LastMessageAt); err != nil {
			return nil, fmt.Errorf("scan public channel row: %w", err)
		}
		c.Kind = domain.Kind(kind)
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
// Consumed only by internal/adapter/notify's narrow MemberLister port, not by
// this context's own application layer.
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

// SetLastReadMessage advances a member's cursor atomically. Comparing the
// per-conversation sequence prevents an out-of-order browser request from
// moving a cursor backwards. Consumed only by internal/usecase/message's
// narrow ConversationReader port, not by this context's own application
// layer (see the design spec's "Cross-context seams").
func (r *ConversationRepository) SetLastReadMessage(ctx context.Context, conversationID, userID, messageID uuid.UUID) error {
	const q = `
		UPDATE conversation_members cm
		SET last_read_message_id = m.id
		FROM messages m
		WHERE cm.conversation_id = $1
		  AND cm.user_id = $2
		  AND m.id = $3
		  AND m.conversation_id = $1
		  AND (cm.last_read_message_id IS NULL OR
		       (SELECT seq FROM messages WHERE id = cm.last_read_message_id) <= m.seq)`
	if _, err := r.db.ExecContext(ctx, q, conversationID, userID, messageID); err != nil {
		return fmt.Errorf("update read cursor: %w", err)
	}
	return nil
}
