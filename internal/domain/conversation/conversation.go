// Package conversation holds the Conversation + Member aggregate: direct
// messages, group chats, and (later) channels, all as one entity keyed by Kind.
package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind discriminates the flavours of a Conversation. direct and group_chat are
// usable now; the channel kinds are defined for schema/permission completeness
// but get their constructors in a later stage.
type Kind string

const (
	KindDirect           Kind = "direct"
	KindGroupChat        Kind = "group_chat"
	KindChannelGroup     Kind = "channel_group"
	KindChannelBroadcast Kind = "channel_broadcast"
)

// Valid reports whether k is one of the four known kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindDirect, KindGroupChat, KindChannelGroup, KindChannelBroadcast:
		return true
	default:
		return false
	}
}

// Role is a member's role within a conversation.
type Role string

const (
	RoleOwner      Role = "owner"
	RoleAdmin      Role = "admin"
	RoleMember     Role = "member"
	RoleSubscriber Role = "subscriber"
)

// Domain errors.
var (
	ErrInvalidKind          = errors.New("conversation: invalid kind")
	ErrInvalidTitle         = errors.New("conversation: title must be 1-256 characters")
	ErrInvalidDirectMembers = errors.New("conversation: direct conversation needs two distinct users")
	ErrNotFound             = errors.New("conversation: not found")
	ErrNotMember            = errors.New("conversation: user is not a member")
)

const maxTitleLen = 256

// Conversation is the shared entity behind DMs, group chats, and channels.
// Title is empty for direct conversations.
type Conversation struct {
	ID        uuid.UUID
	Kind      Kind
	Title     string
	AvatarURL string
	CreatedAt time.Time
}

// Member is a user's membership in a conversation. MutedUntil and
// LastReadMessageID are nil until set.
type Member struct {
	ConversationID    uuid.UUID
	UserID            uuid.UUID
	Role              Role
	JoinedAt          time.Time
	MutedUntil        *time.Time
	LastReadMessageID *uuid.UUID
}

// NewDirect builds a direct conversation between two distinct users. Both
// members get RoleMember and there is no title.
func NewDirect(userA, userB uuid.UUID) (*Conversation, []*Member, error) {
	if userA == userB || userA == uuid.Nil || userB == uuid.Nil {
		return nil, nil, ErrInvalidDirectMembers
	}
	now := time.Now().UTC()
	c := &Conversation{
		ID:        uuid.New(),
		Kind:      KindDirect,
		CreatedAt: now,
	}
	members := []*Member{
		{ConversationID: c.ID, UserID: userA, Role: RoleMember, JoinedAt: now},
		{ConversationID: c.ID, UserID: userB, Role: RoleMember, JoinedAt: now},
	}
	return c, members, nil
}

// NewGroupChat builds a group chat owned by creator. The title is trimmed and
// must be 1-256 characters after trimming.
func NewGroupChat(creator uuid.UUID, title string) (*Conversation, []*Member, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > maxTitleLen {
		return nil, nil, ErrInvalidTitle
	}
	now := time.Now().UTC()
	c := &Conversation{
		ID:        uuid.New(),
		Kind:      KindGroupChat,
		Title:     title,
		CreatedAt: now,
	}
	members := []*Member{
		{ConversationID: c.ID, UserID: creator, Role: RoleOwner, JoinedAt: now},
	}
	return c, members, nil
}

// Repository is the persistence port for the conversation aggregate. It is
// implemented in internal/infrastructure/postgres in a later stage. The method
// set grows as usecases that need AddMember / read-cursor updates arrive.
type Repository interface {
	// Create persists the conversation and its founding members atomically.
	Create(ctx context.Context, c *Conversation, members []*Member) error
	GetByID(ctx context.Context, id uuid.UUID) (*Conversation, error)
	GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*Member, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*Conversation, error)
}
