// internal/conversation/adapter/out/persistence/inbox_repository.go
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	portout "pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

// InboxRepository implements portout.InboxViewPort against the
// conversation_inbox projection. DirectUsername/DirectDisplayName/
// DirectAvatarURL and UnreadCount are resolved with a small live join at
// read time (see the design spec's "Read model scope") rather than
// denormalized into the projection, so they are always exactly current.
type InboxRepository struct {
	db *sql.DB
}

// NewInboxRepository wires the read-side adapter.
func NewInboxRepository(db *sql.DB) *InboxRepository {
	return &InboxRepository{db: db}
}

// ListInbox returns userID's conversations newest-activity-first. A zero
// `before` starts from the newest row; otherwise only rows strictly older
// than the (before, beforeID) cursor are returned. limit is defensively
// clamped; callers pass an already-validated positive value.
func (r *InboxRepository) ListInbox(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*portout.InboxEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var beforePtr *time.Time
	if !before.IsZero() {
		b := before.UTC()
		beforePtr = &b
	}

	const q = `
		SELECT i.conversation_id, i.kind, i.title, i.avatar_url, i.public, i.created_at,
		       i.archived_at, i.last_message_at,
		       peer.username, peer.display_name, peer.avatar_url,
		       COALESCE(unread.count, 0)
		FROM conversation_inbox i
		JOIN conversation_members cm ON cm.conversation_id = i.conversation_id AND cm.user_id = i.user_id
		LEFT JOIN users peer ON peer.id = i.direct_peer_user_id
		LEFT JOIN LATERAL (
			SELECT count(*)::int AS count
			FROM messages msg
			LEFT JOIN messages read_cursor ON read_cursor.id = cm.last_read_message_id
			WHERE msg.conversation_id = i.conversation_id
			  AND msg.type = 'text'
			  AND msg.sender_id IS DISTINCT FROM i.user_id
			  AND msg.seq > COALESCE(read_cursor.seq, 0)
		) unread ON true
		WHERE i.user_id = $1
		  AND i.archived_at IS NULL
		  AND ($2::timestamptz IS NULL OR (i.last_message_at, i.conversation_id) < ($2, $3))
		ORDER BY i.last_message_at DESC, i.conversation_id DESC
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, q, userID, beforePtr, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("query conversation inbox: %w", err)
	}
	defer rows.Close()

	var entries []*portout.InboxEntry
	for rows.Next() {
		var (
			e          portout.InboxEntry
			kind       string
			archivedAt sql.NullTime
			username   sql.NullString
			display    sql.NullString
			avatarURL  sql.NullString
			unread     int
		)
		if err := rows.Scan(&e.ConversationID, &kind, &e.Title, &e.AvatarURL, &e.Public, &e.CreatedAt,
			&archivedAt, &e.LastMessageAt, &username, &display, &avatarURL, &unread); err != nil {
			return nil, fmt.Errorf("scan inbox row: %w", err)
		}
		e.Kind = domain.Kind(kind)
		e.UnreadCount = unread
		if archivedAt.Valid {
			t := archivedAt.Time
			e.ArchivedAt = &t
		}
		if username.Valid {
			e.DirectUsername = username.String
		}
		if display.Valid {
			e.DirectDisplayName = display.String
		}
		if avatarURL.Valid {
			e.DirectAvatarURL = avatarURL.String
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inbox rows: %w", err)
	}
	return entries, nil
}
