package out

import (
	"time"

	"github.com/google/uuid"

	domain "pet-messenger/internal/call/domain"
)

// ICEServer is a WebRTC ICE server entry returned in a join response. It
// lives here (not in internal/infrastructure/ice) so that the ICE provider
// in infrastructure depends inward on this context, keeping the
// application layer free of an infrastructure import.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICEConfigProvider returns the ICE servers a given user should use.
type ICEConfigProvider interface {
	ServersFor(userID uuid.UUID) []ICEServer
}

// JoinInfo is what a client needs to actually connect to a call: the
// media-node ticket, its WebSocket URL, ICE servers, and the role the
// server assigned.
type JoinInfo struct {
	Ticket          string      `json:"ticket"`
	TicketExpiresAt time.Time   `json:"ticket_expires_at"`
	MediaWSURL      string      `json:"media_ws_url"`
	ICEServers      []ICEServer `json:"ice_servers"`
	Role            domain.Role `json:"role"`
}
