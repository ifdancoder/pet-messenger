// internal/conversation/application/port/out/profile_batch_view.go
package out

import (
	"context"

	"github.com/google/uuid"

	domainuser "pet-messenger/internal/user/domain"
)

// ProfileBatchViewPort resolves a batch of user ids to their public
// profiles. Consumer-owned by this context (per the design spec) so that
// conversation's own persistence adapter never has to JOIN the users
// table directly. Implemented by
// internal/user/adapter/out/persistence.UserRepository.
type ProfileBatchViewPort interface {
	GetProfilesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domainuser.Profile, error)
}
