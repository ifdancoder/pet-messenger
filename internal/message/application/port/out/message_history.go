// internal/message/application/port/out/message_history.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// MessageHistoryPort reads a conversation's message history for the
// ListMessages query — both paging directions, since exactly one consumer
// needs both. Implemented by
// internal/message/adapter/out/persistence.MessageRepository.
type MessageHistoryPort interface {
	// ListByConversation returns the conversation's messages ordered by Seq
	// ascending, starting after afterSeq (afterSeq <= 0 means from the first
	// message). Seq is a per-conversation monotonic sequence allocated at
	// commit time under a row lock, so the result is gap-free and in commit
	// order: a client that passes the highest Seq of the contiguous prefix it
	// holds is guaranteed not to miss a message. Because seq is gap-free, a
	// client that receives a Seq more than one above its last contiguous Seq
	// knows it has a gap and must re-fetch from the contiguous watermark.
	// Soft-deleted messages are returned, with Content cleared. A limit <= 0
	// returns domain.ErrInvalidLimit.
	ListByConversation(ctx context.Context, conversationID uuid.UUID, afterSeq int64, limit int) ([]*domainmsg.Message, error)
	// ListBeforeSeq returns the conversation's messages ordered by Seq
	// DESCENDING (newest first), taking those with Seq < beforeSeq; beforeSeq
	// <= 0 means "from the newest message". This is the history-scroll read —
	// opening a conversation (beforeSeq 0) and then paging upward (beforeSeq =
	// the oldest Seq already held). The forward reconnect-catch-up read is
	// ListByConversation. Soft-deleted messages are returned, with Content
	// cleared. A limit <= 0 returns domain.ErrInvalidLimit.
	ListBeforeSeq(ctx context.Context, conversationID uuid.UUID, beforeSeq int64, limit int) ([]*domainmsg.Message, error)
}
