// internal/conversation/application/port/out/user_checker.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainuser "pet-messenger/internal/user/domain"
)

// UserChecker is this context's own narrow out-port for confirming a user
// id resolves to an account, consumer-owned (per the design spec) instead
// of importing user's full UserStorePort, since OpenDirect/AddMember only
// ever need an existence check. Implemented by
// internal/user/adapter/out/persistence.UserRepository.
type UserChecker interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domainuser.User, error)
}
