package in

import (
	"context"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

type AddMemberCommand struct {
	Actor          uuid.UUID
	ConversationID uuid.UUID
	Invitee        uuid.UUID
}

// AddMemberUseCase adds Invitee to a group chat or channel on Actor's
// authority.
type AddMemberUseCase interface {
	Handle(ctx context.Context, cmd AddMemberCommand) (*domain.Member, error)
}
