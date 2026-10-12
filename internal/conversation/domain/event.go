// internal/conversation/domain/event.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Event is implemented by every fact this package publishes for
// asynchronous consumers (the inbox projector in
// internal/conversation/adapter/out/messaging, and any future subscriber).
type Event interface {
	// EventType is the stable wire name used as the JSON envelope's "type"
	// field by internal/conversation/adapter/out/messaging.EventPublisher.
	EventType() string
}

// ConversationCreated is published once, right after a conversation's
// founding row and founding members commit.
type ConversationCreated struct {
	ConversationID uuid.UUID
	Kind           Kind
	ActorID        uuid.UUID
}

// EventType implements Event.
func (ConversationCreated) EventType() string { return "conversation.created" }

// MemberAdded is published whenever a user becomes a member, including each
// founding member of a brand-new conversation. It deliberately carries the
// conversation's own denormalizable fields (Kind, Title, AvatarURL, Public,
// ConversationCreatedAt) so a consumer building a per-user projection (see
// the conversation_inbox read model) never needs a second read to
// materialize a new row. DirectPeerUserID is set only when Kind ==
// KindDirect, to the OTHER participant's id.
type MemberAdded struct {
	ConversationID        uuid.UUID
	UserID                uuid.UUID
	Kind                  Kind
	Title                 string
	AvatarURL             string
	Public                bool
	ConversationCreatedAt time.Time
	DirectPeerUserID      *uuid.UUID
}

// EventType implements Event.
func (MemberAdded) EventType() string { return "member.added" }

// MemberRemoved is published when a user's membership ends, whether by
// leaving or being kicked.
type MemberRemoved struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
}

// EventType implements Event.
func (MemberRemoved) EventType() string { return "member.removed" }

// MemberTierChanged is published on a promotion, demotion, retier, or
// creator claim. It is not consumed by the inbox projector in this slice
// (the inbox projection does not store role/tier) — published for
// completeness and future consumers, matching the project's existing
// practice of recording every lifecycle fact.
type MemberTierChanged struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	NewTier        AdminTier
}

// EventType implements Event.
func (MemberTierChanged) EventType() string { return "member.tier_changed" }

// ConversationArchived is published when a conversation is archived.
type ConversationArchived struct {
	ConversationID uuid.UUID
}

// EventType implements Event.
func (ConversationArchived) EventType() string { return "conversation.archived" }

// ConversationUnarchived is published when a conversation is unarchived.
type ConversationUnarchived struct {
	ConversationID uuid.UUID
}

// EventType implements Event.
func (ConversationUnarchived) EventType() string { return "conversation.unarchived" }
