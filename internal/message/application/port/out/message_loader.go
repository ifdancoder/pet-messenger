// internal/message/application/port/out/message_loader.go

// Package out holds the message application layer's outbound ("driven")
// ports: what the use cases need from the outside world. Implemented by
// internal/message/adapter/out/persistence and
// internal/message/adapter/out/messaging, and (for the two
// conversation-context ports) by internal/conversation/adapter/out/persistence.
package out

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// MessageLoader reads a message for the write side: every command that
// touches an existing message re-derives current state before deciding
// whether an action is allowed. Implemented by
// internal/message/adapter/out/persistence.MessageRepository.
type MessageLoader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domainmsg.Message, error)
	// GetByIdempotencyKey returns the message previously sent to
	// conversationID by senderID with the given client idempotency key, or
	// domain.ErrNotFound if there is none. SendMessage uses it to resolve the
	// ErrDuplicateIdempotencyKey race: a retried send returns the original row
	// instead of a duplicate.
	GetByIdempotencyKey(ctx context.Context, conversationID, senderID uuid.UUID, key string) (*domainmsg.Message, error)
}
