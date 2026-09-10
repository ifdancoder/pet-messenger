// Package call is the call-lifecycle usecase: start (get-or-create) / join /
// end / moderate a call, and handle the media-service's room events. It owns
// the calls rows; the SFU owns who is actually connected.
package call

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

var (
	ErrCallNotSupported = errors.New("call: this conversation kind cannot host a call")
	ErrForbidden        = errors.New("call: not allowed")
	ErrCannotTargetSelf = errors.New("call: cannot target yourself")
	ErrMediaUnavailable = errors.New("call: no media capacity right now")
	ErrMediaNodeGone    = errors.New("call: the media node for this call is gone")
	ErrNoMediaNodes     = errors.New("call: no media nodes registered")
)

type Node struct {
	ID       string
	GRPCAddr string
	WSAddr   string
}

type MediaNodeRegistry interface {
	PickLeastLoaded(ctx context.Context) (Node, error) // ErrNoMediaNodes when empty
	Get(ctx context.Context, nodeID string) (Node, error)
}

type RoomControl interface {
	CreateRoom(ctx context.Context, node Node, callID uuid.UUID) error
	CloseRoom(ctx context.Context, node Node, callID uuid.UUID) error
	MuteParticipant(ctx context.Context, node Node, callID, userID uuid.UUID) error
	RemoveParticipant(ctx context.Context, node Node, callID, userID uuid.UUID) error
}

type JoinTicketIssuer interface {
	Issue(callID, userID uuid.UUID, role domaincall.Role, nodeID string) (token string, expiresAt time.Time, err error)
}

// ICEServer is a WebRTC ICE server entry returned in a join response. It lives
// here (not in internal/infrastructure/ice) so that the ICE provider in
// infrastructure depends inward on the usecase, keeping the usecase free of an
// infrastructure import.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type ICEConfigProvider interface {
	ServersFor(userID uuid.UUID) []ICEServer
}

// SystemEventPoster is re-declared here (rather than imported from
// internal/usecase/conversation) to avoid a usecase→usecase import. Its
// signature matches *message.Service.PostSystemEvent.
type SystemEventPoster interface {
	PostSystemEvent(ctx context.Context, conv *conversation.Conversation, actorID *uuid.UUID, event message.SystemEvent, targetID uuid.UUID, detail string) error
}

type CallNotifier interface {
	CallStarted(ctx context.Context, callID, conversationID, initiatorID uuid.UUID) error
}

type JoinInfo struct {
	Ticket          string          `json:"ticket"`
	TicketExpiresAt time.Time       `json:"ticket_expires_at"`
	MediaWSURL      string          `json:"media_ws_url"`
	ICEServers      []ICEServer     `json:"ice_servers"`
	Role            domaincall.Role `json:"role"`
}
