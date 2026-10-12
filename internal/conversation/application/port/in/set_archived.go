package in

import (
	"context"

	"github.com/google/uuid"
)

type SetArchivedCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	Archived       bool
}

// SetArchivedUseCase archives or unarchives a conversation on Actor's
// authority.
type SetArchivedUseCase interface {
	Handle(ctx context.Context, cmd SetArchivedCommand) error
}
