// internal/message/application/port/in/delete_message.go
package in

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// DeleteMessageCommand is the input to DeleteMessageUseCase.
type DeleteMessageCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
}

// DeleteMessageUseCase soft-deletes the actor's own message (idempotent), or
// (for a non-author with a moderating admin tier) someone else's. It
// returns the tombstoned message (Content cleared, DeletedAt set).
type DeleteMessageUseCase interface {
	Handle(ctx context.Context, cmd DeleteMessageCommand) (*domainmsg.Message, error)
}
