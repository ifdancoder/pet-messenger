// Package domain holds the Conversation + Member aggregate: direct
// messages, group chats, and channels, all as one entity keyed by Kind. It
// is the Entities layer (Clean Architecture) / domain core (Hexagonal) for
// the conversation bounded context — no dependency on anything outside the
// standard library, uuid, and time.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind discriminates the flavours of a Conversation. direct and group_chat are
// usable now; the channel kinds are defined for schema/permission completeness
// but get their constructors in a later stage.
type Kind string

// Conversation kinds.
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

// IsChannel reports whether k is one of the two channel kinds.
func (k Kind) IsChannel() bool {
	return k == KindChannelGroup || k == KindChannelBroadcast
}

// Role is a member's role within a conversation.
type Role string

// Member roles.
const (
	RoleOwner      Role = "owner"
	RoleAdmin      Role = "admin"
	RoleMember     Role = "member"
	RoleSubscriber Role = "subscriber"
)

// Valid reports whether r is one of the four known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleSubscriber:
		return true
	default:
		return false
	}
}

// AdminTier is a member's rung on the moderation ladder, independent of Role.
// TierNone (the zero value) means the member holds no admin powers. Only the
// is_creator row may hold TierCreatorAdmin.
type AdminTier string

// Admin tiers, highest first. Specialized tiers below TierAdmin (content_admin
// and any added later) all share rank 1.
const (
	TierCreatorAdmin AdminTier = "creator_admin"
	TierSuperAdmin   AdminTier = "superadmin"
	TierAdmin        AdminTier = "admin"
	TierContentAdmin AdminTier = "content_admin"
	TierNone         AdminTier = ""
)

// Valid reports whether t is one of the known tiers (TierNone included).
func (t AdminTier) Valid() bool {
	switch t {
	case TierCreatorAdmin, TierSuperAdmin, TierAdmin, TierContentAdmin, TierNone:
		return true
	default:
		return false
	}
}

// rank orders the ladder for the "who may manage whom" checks. Unknown tiers
// and TierNone rank 0.
func (t AdminTier) rank() int {
	switch t {
	case TierCreatorAdmin:
		return 100
	case TierSuperAdmin:
		return 3
	case TierAdmin:
		return 2
	case TierContentAdmin:
		return 1
	default:
		return 0
	}
}

// AtLeast reports whether t sits at or above other on the moderation ladder.
// Cross-package callers use this instead of the unexported rank().
func (t AdminTier) AtLeast(other AdminTier) bool { return t.rank() >= other.rank() }

// Domain errors.
var (
	ErrInvalidKind          = errors.New("conversation: invalid kind")
	ErrInvalidTitle         = errors.New("conversation: title must be 1-256 characters")
	ErrInvalidDirectMembers = errors.New("conversation: direct conversation needs two distinct users")
	ErrInvalidCreator       = errors.New("conversation: creator must be a valid user id")
	ErrNotFound             = errors.New("conversation: not found")
	ErrNotMember            = errors.New("conversation: user is not a member")
	// ErrDirectExists is returned by the write-side adapter's Create when a
	// direct conversation between the same two users already exists
	// (unique-violation on direct_key). The get-or-create direct usecase
	// treats it as "lost a race, re-read".
	ErrDirectExists = errors.New("conversation: a direct conversation between these users already exists")
	// ErrNotChannel is returned when a join / leave / discovery operation
	// targets a conversation that is not a channel_group or channel_broadcast.
	ErrNotChannel = errors.New("conversation: not a channel")
	// ErrConversationArchived is returned when a write (send / join / invite) is
	// attempted against an archived conversation.
	ErrConversationArchived = errors.New("conversation: conversation is archived")
	// ErrCreatorSeatTaken is returned by ClaimCreator when the conversation
	// already has a creator.
	ErrCreatorSeatTaken = errors.New("conversation: this conversation already has a creator")
	// ErrNotCreatorHeir is returned by ClaimCreator when the caller is not the
	// highest-ranked current member of a seat-less conversation.
	ErrNotCreatorHeir = errors.New("conversation: not eligible to claim the creator seat")
)

const maxTitleLen = 256

// Conversation is the shared entity behind DMs, group chats, and channels.
// Title is empty for direct conversations.
type Conversation struct {
	ID        uuid.UUID
	Kind      Kind
	Title     string
	AvatarURL string
	// Public marks a channel as discoverable via ListPublicChannels and
	// self-joinable. Always false for direct conversations and group chats.
	Public bool
	// ArchivedAt is set when the conversation is archived: read-only, hidden
	// from ListForUser and public discovery, history retained. nil = active.
	ArchivedAt *time.Time
	CreatedAt  time.Time
	// LastMessageAt is the denormalized "last activity" timestamp used to sort
	// conversation lists. It equals CreatedAt for a conversation with no
	// messages and is advanced by the message repository as messages are sent.
	LastMessageAt time.Time
	// DirectUsername, DirectDisplayName, and DirectAvatarURL identify the
	// other participant when this conversation is listed for one member of a
	// direct chat.
	DirectUsername    string
	DirectDisplayName string
	DirectAvatarURL   string
	// UnreadCount is the number of text messages from other participants whose
	// sequence is ahead of the listing member's read cursor.
	UnreadCount int
}

// Member is a user's membership in a conversation. MutedUntil and
// LastReadMessageID are nil until set.
type Member struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	Role           Role
	// IsCreator is set once, at conversation creation, on exactly one member.
	// It is immutable and confers nothing on its own — see CanAssignTier /
	// CanManageMember for its only two effects.
	IsCreator bool
	// AdminTier is the member's moderation rung; TierNone when not an admin.
	AdminTier         AdminTier
	JoinedAt          time.Time
	MutedUntil        *time.Time
	LastReadMessageID *uuid.UUID
}

// MemberProfile is a Member row denormalized with the user's display fields,
// for the roster read (GET /conversations/:id/members). The ListMembers query
// assembles it in two steps: MemberViewPort.ListMembers returns the bare
// membership rows, then ProfileBatchViewPort.GetProfilesByIDs resolves their
// display fields in one batch call to the user context, keyed by user ID. A
// user ID with no matching profile (expected to be unreachable today, since
// nothing deletes a user) simply leaves Username / DisplayName / AvatarURL as
// zero values rather than erroring.
type MemberProfile struct {
	Member
	Username    string
	DisplayName string
	AvatarURL   string
}

// NewDirect builds a direct conversation between two distinct users. Both
// members get RoleMember and there is no title.
func NewDirect(userA, userB uuid.UUID) (*Conversation, []*Member, error) {
	if userA == userB || userA == uuid.Nil || userB == uuid.Nil {
		return nil, nil, ErrInvalidDirectMembers
	}
	now := time.Now().UTC()
	c := &Conversation{
		ID:            uuid.Must(uuid.NewV7()),
		Kind:          KindDirect,
		CreatedAt:     now,
		LastMessageAt: now,
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
	if creator == uuid.Nil {
		return nil, nil, ErrInvalidCreator
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > maxTitleLen {
		return nil, nil, ErrInvalidTitle
	}
	now := time.Now().UTC()
	c := &Conversation{
		ID:            uuid.Must(uuid.NewV7()),
		Kind:          KindGroupChat,
		Title:         title,
		CreatedAt:     now,
		LastMessageAt: now,
	}
	members := []*Member{
		{ConversationID: c.ID, UserID: creator, Role: RoleMember, IsCreator: true, AdminTier: TierCreatorAdmin, JoinedAt: now},
	}
	return c, members, nil
}

// NewChannel builds a channel owned by creator. kind must be KindChannelGroup or
// KindChannelBroadcast. The title is trimmed and must be 1-256 characters. A
// public channel is discoverable via the public-channel-view port and
// self-joinable; a private one is only reachable by its members.
func NewChannel(creator uuid.UUID, kind Kind, title string, public bool) (*Conversation, []*Member, error) {
	if creator == uuid.Nil {
		return nil, nil, ErrInvalidCreator
	}
	if !kind.IsChannel() {
		return nil, nil, ErrInvalidKind
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > maxTitleLen {
		return nil, nil, ErrInvalidTitle
	}
	now := time.Now().UTC()
	c := &Conversation{
		ID:            uuid.Must(uuid.NewV7()),
		Kind:          kind,
		Title:         title,
		Public:        public,
		CreatedAt:     now,
		LastMessageAt: now,
	}
	members := []*Member{
		{ConversationID: c.ID, UserID: creator, Role: RoleMember, IsCreator: true, AdminTier: TierCreatorAdmin, JoinedAt: now},
	}
	return c, members, nil
}

// CanPost reports whether m may send messages to c. Pass a Member fetched for
// THIS conversation, never a member from a request body or a different
// conversation. In direct, group_chat and channel_group anyone but a
// subscriber may post; in channel_broadcast only the creator or an admin
// (any tier) may.
func (c *Conversation) CanPost(m *Member) bool {
	switch c.Kind {
	case KindDirect, KindGroupChat, KindChannelGroup:
		return m.Role != RoleSubscriber
	case KindChannelBroadcast:
		return m.IsCreator || m.AdminTier != TierNone
	default:
		return false
	}
}

// CanModerateMessages reports whether m may delete another member's message.
// Every admin tier (content_admin and up) can; plain members and subscribers
// cannot.
func (m *Member) CanModerateMessages() bool {
	return m.AdminTier.rank() >= 1
}

// CanAddMembers reports whether actor may add another user to c. Never for a
// direct conversation; otherwise an admin-tier holder (rank >= 2) may.
func (c *Conversation) CanAddMembers(actor *Member) bool {
	return c.Kind != KindDirect && actor.AdminTier.rank() >= 2
}

// IsArchived reports whether the conversation has been archived.
func (c *Conversation) IsArchived() bool { return c.ArchivedAt != nil }

// CanManageMember reports whether actor may kick or retier target. No one may
// manage the creator or themselves. A rank-3+ actor (superadmin / creator_admin)
// manages anyone strictly lower in rank; a rank-2 actor (admin) manages only
// members that hold no tier; anyone lower manages no one.
func CanManageMember(actor, target *Member) bool {
	if target.IsCreator || actor.UserID == target.UserID {
		return false
	}
	switch {
	case actor.AdminTier.rank() >= TierSuperAdmin.rank():
		return actor.AdminTier.rank() > target.AdminTier.rank()
	case actor.AdminTier.rank() == TierAdmin.rank():
		return target.AdminTier.rank() == 0
	default:
		return false
	}
}

// CanArchive reports whether m may archive or unarchive the conversation.
// superadmin and creator_admin only — the same authority tier that manages
// admins.
func CanArchive(m *Member) bool {
	return m.AdminTier.rank() >= TierSuperAdmin.rank()
}

// CanAssignTier reports whether actor may set target's tier to newTier.
//
// Self: a member may always lower their own tier (or leave it unchanged); only
// the creator may raise their own, and only to TierCreatorAdmin.
// Others: rank-3+ only, target must be manageable (CanManageMember), newTier
// must be a real tier strictly below the actor's rank, and TierCreatorAdmin is
// never assignable to another member.
func CanAssignTier(actor, target *Member, newTier AdminTier) bool {
	if !newTier.Valid() {
		return false
	}
	if actor.UserID == target.UserID {
		if newTier == TierCreatorAdmin {
			return actor.IsCreator
		}
		return newTier.rank() <= actor.AdminTier.rank()
	}
	if newTier == TierCreatorAdmin {
		return false
	}
	return actor.AdminTier.rank() >= TierSuperAdmin.rank() &&
		CanManageMember(actor, target) &&
		newTier.rank() < actor.AdminTier.rank()
}

// IsMuted reports whether the member has muted the conversation at time now.
func (m *Member) IsMuted(now time.Time) bool {
	return m.MutedUntil != nil && now.Before(*m.MutedUntil)
}
