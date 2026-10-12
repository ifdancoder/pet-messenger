// internal/conversation/adapter/out/persistence/inbox_repository_writes.go
package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

// InboxRow is the write-side shape for materializing one conversation_inbox
// row from a MemberAdded event.
type InboxRow struct {
	UserID, ConversationID uuid.UUID
	Kind                   domain.Kind
	Title, AvatarURL       string
	Public                 bool
	CreatedAt              time.Time
	DirectPeerUserID       *uuid.UUID
	EventSeq               int64
}

// UpsertMember inserts one conversation_inbox row. Naturally idempotent
// under JetStream redelivery (ON CONFLICT DO NOTHING) — an insert never
// regresses an existing row, so no seq guard is needed here.
func (r *InboxRepository) UpsertMember(ctx context.Context, row InboxRow) error {
	const q = `
		INSERT INTO conversation_inbox (user_id, conversation_id, kind, title, avatar_url, public, created_at, direct_peer_user_id, last_message_at, last_event_seq)
		VALUES ($1, $2, $3::conversation_kind, $4, $5, $6, $7, $8, $7, $9)
		ON CONFLICT (user_id, conversation_id) DO NOTHING`
	if _, err := r.db.ExecContext(ctx, q, row.UserID, row.ConversationID, string(row.Kind), row.Title, row.AvatarURL, row.Public, row.CreatedAt, nullableUUID(row.DirectPeerUserID), row.EventSeq); err != nil {
		return fmt.Errorf("upsert conversation inbox row: %w", err)
	}
	return nil
}

// RemoveMember deletes one conversation_inbox row, guarded by eventSeq so a
// stale (lower-seq) redelivery cannot delete a row a newer event has since
// updated (e.g. the user left and rejoined).
func (r *InboxRepository) RemoveMember(ctx context.Context, conversationID, userID uuid.UUID, eventSeq int64) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM conversation_inbox WHERE user_id = $1 AND conversation_id = $2 AND last_event_seq < $3`,
		userID, conversationID, eventSeq,
	); err != nil {
		return fmt.Errorf("delete conversation inbox row: %w", err)
	}
	return nil
}

// UpdateArchived sets (or, with a nil archivedAt, clears) every row's
// archived_at for conversationID, guarded by eventSeq against out-of-order
// archive/unarchive delivery.
func (r *InboxRepository) UpdateArchived(ctx context.Context, conversationID uuid.UUID, archivedAt *time.Time, eventSeq int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE conversation_inbox SET archived_at = $2, last_event_seq = $3, updated_at = now() WHERE conversation_id = $1 AND last_event_seq < $3`,
		conversationID, archivedAt, eventSeq,
	); err != nil {
		return fmt.Errorf("update conversation inbox archived_at: %w", err)
	}
	return nil
}

// UpdateLastMessageAt advances every row's last_message_at for
// conversationID. Guarded by the column's own value rather than
// last_event_seq — message.created events arrive on the existing core-NATS
// msg.> subject (no JetStream stream sequence), so lastMessageAt itself is
// the monotonic guard: redelivery-free core NATS means the only ordering
// risk is concurrent sends interleaving, and a strictly-greater timestamp
// check handles that the same way the pre-retrofit MessageRepository.Create
// column (GREATEST(last_message_at, ...)) did.
func (r *InboxRepository) UpdateLastMessageAt(ctx context.Context, conversationID uuid.UUID, lastMessageAt time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE conversation_inbox SET last_message_at = $2, updated_at = now() WHERE conversation_id = $1 AND last_message_at < $2`,
		conversationID, lastMessageAt,
	); err != nil {
		return fmt.Errorf("update conversation inbox last_message_at: %w", err)
	}
	return nil
}
