// internal/message/application/port/out/message_writer.go
package out

import (
	"context"

	domainmsg "pet-messenger/internal/message/domain"
)

// MessageWriter is the write side of the message aggregate. Implemented by
// internal/message/adapter/out/persistence.MessageRepository.
type MessageWriter interface {
	// Create persists the message, allocating and setting m.Seq (the
	// per-conversation monotonic sequence) as part of the write. If
	// m.IdempotencyKey is set and a message with that (ConversationID,
	// SenderID, key) already exists, Create returns
	// domain.ErrDuplicateIdempotencyKey and the whole transaction — including
	// the seq allocation — is rolled back, so no seq is consumed.
	Create(ctx context.Context, m *domainmsg.Message) error
	// Update persists a mutated message (Edit / MarkDeleted).
	Update(ctx context.Context, m *domainmsg.Message) error
}
