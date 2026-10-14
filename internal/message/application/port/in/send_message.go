// internal/message/application/port/in/send_message.go

// Package in holds the message application layer's inbound ("driving")
// ports: one use-case interface per command/query, each with its own
// parameter struct.
package in

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// SendMessageCommand is the input to SendMessageUseCase.
type SendMessageCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	Content        string
	// ReplyToID is optional; when set it must reference a message in the same
	// conversation.
	ReplyToID *uuid.UUID
	// IdempotencyKey is optional ("" = none): a retry with the same key
	// returns the original message instead of creating a duplicate.
	IdempotencyKey string
}

// SendMessageUseCase posts a text message.
type SendMessageUseCase interface {
	Handle(ctx context.Context, cmd SendMessageCommand) (*domainmsg.Message, error)
}
