// Package call holds the Call aggregate: one live audio/video session in a
// conversation, its media-node assignment, and its participants. The SFU in
// media-service is the source of truth for who is *connected*; these rows are
// the durable record for history, the "one live call per conversation"
// invariant, and the join-authorization check.
package call

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive Status = "active"
	StatusEnded  Status = "ended"
)

func (s Status) Valid() bool { return s == StatusActive || s == StatusEnded }

type Role string

const (
	RolePublisher Role = "publisher"
	RoleViewer    Role = "viewer"
)

func (r Role) Valid() bool { return r == RolePublisher || r == RoleViewer }

var (
	ErrInvalidConversation = errors.New("call: conversation must be a valid id")
	ErrInvalidInitiator    = errors.New("call: initiator must be a valid id")
	ErrInvalidCall         = errors.New("call: call must be a valid id")
	ErrInvalidNode         = errors.New("call: media node id must be non-empty")
	ErrInvalidRole         = errors.New("call: unknown participant role")
	ErrNotFound            = errors.New("call: not found")
	ErrAlreadyEnded        = errors.New("call: already ended")
	ErrCallNotLive         = errors.New("call: call is not live")
	ErrAlreadyInCall       = errors.New("call: user is already a live participant")
	ErrParticipantNotFound = errors.New("call: participant not found")
	ErrLiveCallExists      = errors.New("call: a live call already exists for this conversation")
)

type Call struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	InitiatorID    uuid.UUID
	Status         Status
	MediaNodeID    string
	StartedAt      time.Time
	EndedAt        *time.Time
}

func New(conversationID, initiatorID uuid.UUID, nodeID string) (*Call, error) {
	if conversationID == uuid.Nil {
		return nil, ErrInvalidConversation
	}
	if initiatorID == uuid.Nil {
		return nil, ErrInvalidInitiator
	}
	if nodeID == "" {
		return nil, ErrInvalidNode
	}
	return &Call{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: conversationID,
		InitiatorID:    initiatorID,
		Status:         StatusActive,
		MediaNodeID:    nodeID,
		StartedAt:      time.Now().UTC(),
	}, nil
}

func (c *Call) IsLive() bool { return c.Status == StatusActive }

// End marks the call ended. It returns ErrAlreadyEnded if the call was not
// live, so the two paths that can end a call (EndCall usecase, the
// call.<id>.ended NATS event) do not double-post the timeline message.
func (c *Call) End() error {
	if !c.IsLive() {
		return ErrAlreadyEnded
	}
	now := time.Now().UTC()
	c.Status = StatusEnded
	c.EndedAt = &now
	return nil
}

type Participant struct {
	ID       uuid.UUID
	CallID   uuid.UUID
	UserID   uuid.UUID
	Role     Role
	JoinedAt time.Time
	LeftAt   *time.Time
}

func NewParticipant(callID, userID uuid.UUID, role Role) (*Participant, error) {
	if callID == uuid.Nil {
		return nil, ErrInvalidCall
	}
	if userID == uuid.Nil {
		return nil, ErrInvalidInitiator
	}
	if !role.Valid() {
		return nil, ErrInvalidRole
	}
	return &Participant{
		ID:       uuid.Must(uuid.NewV7()),
		CallID:   callID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now().UTC(),
	}, nil
}
