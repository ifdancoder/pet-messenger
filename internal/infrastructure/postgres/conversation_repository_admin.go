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

// rankExpr orders conversation_members by effective authority for succession /
// claim: creator_admin > superadmin > admin > content_admin > none.
const rankExpr = `CASE admin_tier
	WHEN 'creator_admin' THEN 100
	WHEN 'superadmin' THEN 3
	WHEN 'admin' THEN 2
	WHEN 'content_admin' THEN 1
	ELSE 0 END`

func (r *ConversationRepository) SetArchived(ctx context.Context, conversationID uuid.UUID, archived bool) error {
	var ts any // nil -> SQL NULL (unarchive)
	if archived {
		ts = time.Now().UTC()
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET archived_at = $2 WHERE id = $1`,
		conversationID, ts,
	)
	if err != nil {
		return fmt.Errorf("update conversation archived_at: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return conversation.ErrNotFound
	}
	return nil
}

func (r *ConversationRepository) RemoveMemberWithSuccession(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Serialize all succession / claim activity for this conversation: take the
	// parent row lock first (conversations-then-members, matching
	// MessageRepository.Create) so a concurrent leaver cannot delete the heir
	// row out from under the LIMIT 1 ... FOR UPDATE below. A missing id simply
	// locks nothing — the usecase's GetByID has already 404'd.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM conversations WHERE id = $1 FOR UPDATE`, conversationID); err != nil {
		return nil, fmt.Errorf("lock conversation: %w", err)
	}

	var wasCreator bool
	err = tx.QueryRowContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2 RETURNING is_creator`,
		conversationID, userID,
	).Scan(&wasCreator)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("delete member: %w", err)
	}

	if !wasCreator {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return nil, nil
	}

	var heir uuid.UUID
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM conversation_members
		 WHERE conversation_id = $1
		 ORDER BY `+rankExpr+` DESC, joined_at ASC, user_id ASC
		 LIMIT 1 FOR UPDATE`,
		conversationID,
	).Scan(&heir)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return nil, nil // no members remain
	}
	if err != nil {
		return nil, fmt.Errorf("select heir: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE conversation_members SET is_creator = true, admin_tier = 'creator_admin'
		 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, heir,
	); err != nil {
		return nil, fmt.Errorf("promote heir: %w", err)
	}

	promoted, err := scanMemberTx(ctx, tx, conversationID, heir)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return promoted, nil
}

func (r *ConversationRepository) ClaimCreator(ctx context.Context, conversationID, userID uuid.UUID) (*conversation.Member, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Lock the parent conversation row first so concurrent claims serialize:
	// under READ COMMITTED the seat-check SELECT below takes no gap lock when no
	// creator row exists, so without this two claimants could both pass it.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM conversations WHERE id = $1 FOR UPDATE`, conversationID); err != nil {
		return nil, fmt.Errorf("lock conversation: %w", err)
	}

	var one int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM conversation_members WHERE conversation_id = $1 AND is_creator FOR UPDATE`,
		conversationID,
	).Scan(&one)
	if err == nil {
		return nil, conversation.ErrCreatorSeatTaken
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check creator seat: %w", err)
	}

	var heir uuid.UUID
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM conversation_members
		 WHERE conversation_id = $1
		 ORDER BY `+rankExpr+` DESC, joined_at ASC, user_id ASC
		 LIMIT 1`,
		conversationID,
	).Scan(&heir)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, conversation.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("select heir: %w", err)
	}
	if heir != userID {
		return nil, conversation.ErrNotCreatorHeir
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE conversation_members SET is_creator = true, admin_tier = 'creator_admin'
		 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	); err != nil {
		// Backstop for a concurrent claim that slipped past the seat check: the
		// partial unique index rejects the second is_creator row.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_conversation_members_one_creator" {
			return nil, conversation.ErrCreatorSeatTaken
		}
		return nil, fmt.Errorf("claim creator: %w", err)
	}

	claimed, err := scanMemberTx(ctx, tx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return claimed, nil
}

func (r *ConversationRepository) ListMembers(ctx context.Context, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*conversation.MemberProfile, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var beforePtr *time.Time
	if !before.IsZero() {
		b := before.UTC()
		beforePtr = &b
	}
	const q = `
		SELECT m.conversation_id, m.user_id, m.role, m.joined_at, m.muted_until,
		       m.last_read_message_id, m.is_creator, m.admin_tier,
		       u.username, u.display_name, u.avatar_url
		FROM conversation_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.conversation_id = $1
		  AND ($2::bool = false OR m.admin_tier IS NOT NULL OR m.is_creator)
		  AND ($3::timestamptz IS NULL OR (m.joined_at, m.user_id) > ($3, $4))
		ORDER BY m.joined_at ASC, m.user_id ASC
		LIMIT $5`
	rows, err := r.db.QueryContext(ctx, q, conversationID, adminsOnly, beforePtr, beforeUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var out []*conversation.MemberProfile
	for rows.Next() {
		var (
			mp         conversation.MemberProfile
			role       string
			mutedUntil sql.NullTime
			lastRead   uuid.NullUUID
			adminTier  sql.NullString
		)
		if err := rows.Scan(
			&mp.ConversationID, &mp.UserID, &role, &mp.JoinedAt, &mutedUntil,
			&lastRead, &mp.IsCreator, &adminTier,
			&mp.Username, &mp.DisplayName, &mp.AvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan member profile: %w", err)
		}
		mp.Role = conversation.Role(role)
		if mutedUntil.Valid {
			t := mutedUntil.Time
			mp.MutedUntil = &t
		}
		if lastRead.Valid {
			id := lastRead.UUID
			mp.LastReadMessageID = &id
		}
		if adminTier.Valid {
			mp.AdminTier = conversation.AdminTier(adminTier.String)
		} else {
			mp.AdminTier = conversation.TierNone
		}
		out = append(out, &mp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate member rows: %w", err)
	}
	return out, nil
}

// scanMemberTx reads one membership row inside a transaction (used after a
// promotion to return the fresh row).
func scanMemberTx(ctx context.Context, tx *sql.Tx, conversationID, userID uuid.UUID) (*conversation.Member, error) {
	var (
		m          conversation.Member
		role       string
		mutedUntil sql.NullTime
		lastRead   uuid.NullUUID
		adminTier  sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`SELECT conversation_id, user_id, role, joined_at, muted_until, last_read_message_id, is_creator, admin_tier
		 FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&m.ConversationID, &m.UserID, &role, &m.JoinedAt, &mutedUntil, &lastRead, &m.IsCreator, &adminTier)
	if err != nil {
		return nil, fmt.Errorf("scan member: %w", err)
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
	if adminTier.Valid {
		m.AdminTier = conversation.AdminTier(adminTier.String)
	} else {
		m.AdminTier = conversation.TierNone
	}
	return &m, nil
}
