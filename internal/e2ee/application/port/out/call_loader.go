package out

import (
	"context"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
)

// CallLoader reads a call by id, used to confirm it is live and to find its
// initiator/conversation before authorizing a call key-envelope operation.
// Consumer-owned by this context - narrower than importing call's full
// pre-retrofit Repository, since this context only ever needs this one
// method. Implemented today by
// internal/infrastructure/postgres.CallRepository (the `call` bounded
// context has not been retrofitted onto this project's new structure yet;
// when it is, only the concrete type satisfying this interface changes,
// not this port's declaration).
type CallLoader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domaincall.Call, error)
}
