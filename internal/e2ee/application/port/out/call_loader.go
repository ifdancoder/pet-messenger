package out

import (
	"context"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/call/domain"
)

// CallLoader reads a call by id, used to confirm it is live and to find its
// initiator/conversation before authorizing a call key-envelope operation.
// Consumer-owned by this context -- narrower than importing call's own
// CallWriter/ParticipantWriter/ParticipantLister ports, since this context
// only ever needs this one method. Implemented today by
// internal/call/adapter/out/persistence.CallRepository.
type CallLoader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domaincall.Call, error)
}
