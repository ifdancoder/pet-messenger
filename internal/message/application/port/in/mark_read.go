// internal/message/application/port/in/mark_read.go
package in

import (
	"context"

	"github.com/google/uuid"
)

// MarkReadCommand is the input to MarkReadUseCase.
type MarkReadCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
}

// MarkReadUseCase records that the actor has displayed MessageID and every
// earlier message in the conversation.
type MarkReadUseCase interface {
	Handle(ctx context.Context, cmd MarkReadCommand) error
}
