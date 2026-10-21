package out

import (
	"time"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// JoinTicketIssuer mints a short-lived ticket the client presents to the
// media node's WebSocket endpoint to join a call.
type JoinTicketIssuer interface {
	Issue(callID, userID uuid.UUID, role domain.Role, nodeID string) (token string, expiresAt time.Time, err error)
}
