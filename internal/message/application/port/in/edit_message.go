// internal/message/application/port/in/edit_message.go
package in

import (
	"context"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

// EditMessageCommand is the input to EditMessageUseCase.
type EditMessageCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	Content        string
}

// EditMessageUseCase replaces the content of the actor's own text message.
// The message must belong to the given conversation and the actor must be
// both a current member and the original sender.
type EditMessageUseCase interface {
	Handle(ctx context.Context, cmd EditMessageCommand) (*domainmsg.Message, error)
}
