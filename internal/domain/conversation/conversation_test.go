package conversation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
)

func TestKindValid(t *testing.T) {
	tests := []struct {
		kind conversation.Kind
		want bool
	}{
		{conversation.KindDirect, true},
		{conversation.KindGroupChat, true},
		{conversation.KindChannelGroup, true},
		{conversation.KindChannelBroadcast, true},
		{conversation.Kind("bogus"), false},
		{conversation.Kind(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := tt.kind.Valid(); got != tt.want {
				t.Errorf("Kind(%q).Valid() = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestRoleValid(t *testing.T) {
	tests := []struct {
		role conversation.Role
		want bool
	}{
		{conversation.RoleOwner, true},
		{conversation.RoleAdmin, true},
		{conversation.RoleMember, true},
		{conversation.RoleSubscriber, true},
		{conversation.Role("guest"), false},
		{conversation.Role(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.Valid(); got != tt.want {
				t.Errorf("Role(%q).Valid() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestNewDirect(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	c, members, err := conversation.NewDirect(a, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Kind != conversation.KindDirect {
		t.Errorf("Kind = %q, want %q", c.Kind, conversation.KindDirect)
	}
	if c.Title != "" {
		t.Errorf("Title = %q, want empty", c.Title)
	}
	if c.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if c.ID.Version() != 7 {
		t.Errorf("ID version = %d, want 7 (UUIDv7)", c.ID.Version())
	}
	if c.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if c.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt.Location() = %v, want %v", c.CreatedAt.Location(), time.UTC)
	}
	if !c.LastMessageAt.Equal(c.CreatedAt) {
		t.Errorf("LastMessageAt = %v, want == CreatedAt %v", c.LastMessageAt, c.CreatedAt)
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}
	seen := map[uuid.UUID]bool{}
	for _, m := range members {
		if m.ConversationID != c.ID {
			t.Errorf("member ConversationID = %v, want %v", m.ConversationID, c.ID)
		}
		if m.Role != conversation.RoleMember {
			t.Errorf("member Role = %q, want %q", m.Role, conversation.RoleMember)
		}
		if m.JoinedAt.IsZero() {
			t.Error("expected member JoinedAt to be set")
		}
		if m.JoinedAt.Location() != time.UTC {
			t.Errorf("member JoinedAt.Location() = %v, want %v", m.JoinedAt.Location(), time.UTC)
		}
		if !m.JoinedAt.Equal(c.CreatedAt) {
			t.Errorf("member JoinedAt = %v, want == CreatedAt %v", m.JoinedAt, c.CreatedAt)
		}
		seen[m.UserID] = true
	}
	if !seen[a] || !seen[b] {
		t.Errorf("members %+v do not cover both users %v, %v", members, a, b)
	}
}

func TestNewDirectInvalidMembers(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	tests := []struct {
		name         string
		userA, userB uuid.UUID
	}{
		{"identical users", a, a},
		{"nil userA", uuid.Nil, b},
		{"nil userB", a, uuid.Nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := conversation.NewDirect(tt.userA, tt.userB)
			if !errors.Is(err, conversation.ErrInvalidDirectMembers) {
				t.Fatalf("err = %v, want ErrInvalidDirectMembers", err)
			}
		})
	}
}

func TestNewGroupChat(t *testing.T) {
	creator := uuid.New()

	c, members, err := conversation.NewGroupChat(creator, "  Team Standup  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Kind != conversation.KindGroupChat {
		t.Errorf("Kind = %q, want %q", c.Kind, conversation.KindGroupChat)
	}
	if c.Title != "Team Standup" {
		t.Errorf("Title = %q, want %q (trimmed)", c.Title, "Team Standup")
	}
	if len(members) != 1 {
		t.Fatalf("len(members) = %d, want 1", len(members))
	}
	if members[0].UserID != creator {
		t.Errorf("member UserID = %v, want %v", members[0].UserID, creator)
	}
	if members[0].Role != conversation.RoleMember || !members[0].IsCreator || members[0].AdminTier != conversation.TierCreatorAdmin {
		t.Errorf("founder = %+v, want RoleMember + IsCreator + TierCreatorAdmin", members[0])
	}
	if members[0].ConversationID != c.ID {
		t.Errorf("member ConversationID = %v, want %v", members[0].ConversationID, c.ID)
	}
	if c.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt.Location() = %v, want %v", c.CreatedAt.Location(), time.UTC)
	}
	if !c.LastMessageAt.Equal(c.CreatedAt) {
		t.Errorf("LastMessageAt = %v, want == CreatedAt %v", c.LastMessageAt, c.CreatedAt)
	}
	if members[0].JoinedAt.Location() != time.UTC {
		t.Errorf("member JoinedAt.Location() = %v, want %v", members[0].JoinedAt.Location(), time.UTC)
	}
	if !members[0].JoinedAt.Equal(c.CreatedAt) {
		t.Errorf("member JoinedAt = %v, want == CreatedAt %v", members[0].JoinedAt, c.CreatedAt)
	}
}

func TestNewGroupChatInvalidTitle(t *testing.T) {
	creator := uuid.New()
	tests := []struct {
		name  string
		title string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"too long", strings.Repeat("x", 257)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := conversation.NewGroupChat(creator, tt.title)
			if !errors.Is(err, conversation.ErrInvalidTitle) {
				t.Fatalf("err = %v, want ErrInvalidTitle for %s title", err, tt.name)
			}
		})
	}
}

func TestNewGroupChatNilCreator(t *testing.T) {
	_, _, err := conversation.NewGroupChat(uuid.Nil, "Team")
	if !errors.Is(err, conversation.ErrInvalidCreator) {
		t.Fatalf("err = %v, want ErrInvalidCreator", err)
	}
}

func TestNewGroupChatTitleBoundary(t *testing.T) {
	creator := uuid.New()
	if _, _, err := conversation.NewGroupChat(creator, strings.Repeat("x", 256)); err != nil {
		t.Fatalf("256-char title should be valid, got %v", err)
	}
}

func TestConstantValues(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"KindDirect", string(conversation.KindDirect), "direct"},
		{"KindGroupChat", string(conversation.KindGroupChat), "group_chat"},
		{"KindChannelGroup", string(conversation.KindChannelGroup), "channel_group"},
		{"KindChannelBroadcast", string(conversation.KindChannelBroadcast), "channel_broadcast"},
		{"RoleOwner", string(conversation.RoleOwner), "owner"},
		{"RoleAdmin", string(conversation.RoleAdmin), "admin"},
		{"RoleMember", string(conversation.RoleMember), "member"},
		{"RoleSubscriber", string(conversation.RoleSubscriber), "subscriber"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestCanPost(t *testing.T) {
	// want[kind] -> for a plain member (Role) and for an admin-tier holder.
	cases := []struct {
		kind        conversation.Kind
		member      *conversation.Member
		wantCanPost bool
	}{
		{conversation.KindDirect, &conversation.Member{Role: conversation.RoleMember}, true},
		{conversation.KindDirect, &conversation.Member{Role: conversation.RoleSubscriber}, false},
		{conversation.KindGroupChat, &conversation.Member{Role: conversation.RoleMember}, true},
		{conversation.KindGroupChat, &conversation.Member{Role: conversation.RoleSubscriber}, false},
		{conversation.KindChannelGroup, &conversation.Member{Role: conversation.RoleMember}, true},
		{conversation.KindChannelGroup, &conversation.Member{Role: conversation.RoleSubscriber}, false},
		{conversation.KindChannelBroadcast, &conversation.Member{Role: conversation.RoleMember}, false},
		{conversation.KindChannelBroadcast, &conversation.Member{Role: conversation.RoleSubscriber}, false},
		{conversation.KindChannelBroadcast, &conversation.Member{Role: conversation.RoleMember, IsCreator: true}, true},
		{conversation.KindChannelBroadcast, &conversation.Member{Role: conversation.RoleMember, AdminTier: conversation.TierContentAdmin}, true},
		{conversation.KindChannelBroadcast, &conversation.Member{Role: conversation.RoleMember, AdminTier: conversation.TierSuperAdmin}, true},
	}
	for _, tc := range cases {
		c := &conversation.Conversation{Kind: tc.kind}
		if got := c.CanPost(tc.member); got != tc.wantCanPost {
			t.Errorf("Conversation{Kind:%q}.CanPost(%+v) = %v, want %v", tc.kind, tc.member, got, tc.wantCanPost)
		}
	}
	if (&conversation.Conversation{Kind: conversation.Kind("bogus")}).CanPost(&conversation.Member{}) {
		t.Error("CanPost on unknown kind should be false")
	}
}

func TestNewChannel(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	for _, kind := range []conversation.Kind{conversation.KindChannelGroup, conversation.KindChannelBroadcast} {
		c, members, err := conversation.NewChannel(creator, kind, "  Announcements  ", true)
		if err != nil {
			t.Fatalf("NewChannel(%s): %v", kind, err)
		}
		if c.Kind != kind || c.Title != "Announcements" || !c.Public {
			t.Errorf("channel = %+v", c)
		}
		if c.ID.Version() != 7 || c.CreatedAt.IsZero() || !c.LastMessageAt.Equal(c.CreatedAt) {
			t.Errorf("channel timestamps/id wrong: %+v", c)
		}
		if len(members) != 1 || members[0].UserID != creator ||
			members[0].Role != conversation.RoleMember || !members[0].IsCreator ||
			members[0].AdminTier != conversation.TierCreatorAdmin {
			t.Errorf("members = %+v", members)
		}
	}
}

func TestNewChannel_PrivateDefault(t *testing.T) {
	c, _, err := conversation.NewChannel(uuid.Must(uuid.NewV7()), conversation.KindChannelGroup, "x", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if c.Public {
		t.Error("Public should be false when public=false")
	}
}

func TestNewChannel_Invalid(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	tests := []struct {
		name    string
		creator uuid.UUID
		kind    conversation.Kind
		title   string
		wantErr error
	}{
		{"nil creator", uuid.Nil, conversation.KindChannelGroup, "x", conversation.ErrInvalidCreator},
		{"kind direct", creator, conversation.KindDirect, "x", conversation.ErrInvalidKind},
		{"kind group_chat", creator, conversation.KindGroupChat, "x", conversation.ErrInvalidKind},
		{"blank title", creator, conversation.KindChannelBroadcast, "   ", conversation.ErrInvalidTitle},
		{"title too long", creator, conversation.KindChannelGroup, strings.Repeat("x", 257), conversation.ErrInvalidTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := conversation.NewChannel(tt.creator, tt.kind, tt.title, false)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestKind_IsChannel(t *testing.T) {
	for k, want := range map[conversation.Kind]bool{
		conversation.KindDirect:           false,
		conversation.KindGroupChat:        false,
		conversation.KindChannelGroup:     true,
		conversation.KindChannelBroadcast: true,
		conversation.Kind("bogus"):        false,
	} {
		if got := k.IsChannel(); got != want {
			t.Errorf("Kind(%q).IsChannel() = %v, want %v", k, got, want)
		}
	}
}

func TestAdminTier_ValidAndRankOrder(t *testing.T) {
	for _, ok := range []conversation.AdminTier{
		conversation.TierCreatorAdmin, conversation.TierSuperAdmin,
		conversation.TierAdmin, conversation.TierContentAdmin, conversation.TierNone,
	} {
		if !ok.Valid() {
			t.Errorf("%q should be Valid", ok)
		}
	}
	if conversation.AdminTier("mod").Valid() {
		t.Error(`AdminTier("mod") should be invalid`)
	}
}

func TestCanModerateMessages(t *testing.T) {
	for tier, want := range map[conversation.AdminTier]bool{
		conversation.TierNone:         false,
		conversation.TierContentAdmin: true,
		conversation.TierAdmin:        true,
		conversation.TierSuperAdmin:   true,
		conversation.TierCreatorAdmin: true,
	} {
		m := &conversation.Member{AdminTier: tier}
		if got := m.CanModerateMessages(); got != want {
			t.Errorf("tier %q: CanModerateMessages() = %v, want %v", tier, got, want)
		}
	}
}

func TestCanAddMembers(t *testing.T) {
	admin := &conversation.Member{AdminTier: conversation.TierAdmin}
	plain := &conversation.Member{AdminTier: conversation.TierNone}
	content := &conversation.Member{AdminTier: conversation.TierContentAdmin}
	if (&conversation.Conversation{Kind: conversation.KindDirect}).CanAddMembers(admin) {
		t.Error("no one may add members to a direct conversation")
	}
	gc := &conversation.Conversation{Kind: conversation.KindGroupChat}
	if !gc.CanAddMembers(admin) {
		t.Error("an admin may add members to a group chat")
	}
	if gc.CanAddMembers(plain) || gc.CanAddMembers(content) {
		t.Error("a plain member / content-admin may not add members")
	}
}

func TestCanManageMember(t *testing.T) {
	mk := func(tier conversation.AdminTier, creator bool) *conversation.Member {
		return &conversation.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: tier, IsCreator: creator}
	}
	creatorAdmin := mk(conversation.TierCreatorAdmin, true)
	super1 := mk(conversation.TierSuperAdmin, false)
	super2 := mk(conversation.TierSuperAdmin, false)
	admin := mk(conversation.TierAdmin, false)
	content := mk(conversation.TierContentAdmin, false)
	plain := mk(conversation.TierNone, false)

	// creator_admin manages everyone below.
	for _, target := range []*conversation.Member{super1, admin, content, plain} {
		if !conversation.CanManageMember(creatorAdmin, target) {
			t.Errorf("creator_admin should manage %q", target.AdminTier)
		}
	}
	// superadmin manages strictly lower, not another superadmin.
	if !conversation.CanManageMember(super1, admin) || !conversation.CanManageMember(super1, content) || !conversation.CanManageMember(super1, plain) {
		t.Error("superadmin should manage admin/content/plain")
	}
	if conversation.CanManageMember(super1, super2) {
		t.Error("superadmin must not manage another superadmin")
	}
	// admin manages only plain members.
	if !conversation.CanManageMember(admin, plain) {
		t.Error("admin should manage a plain member")
	}
	if conversation.CanManageMember(admin, content) || conversation.CanManageMember(admin, super1) {
		t.Error("admin must not manage any tier holder")
	}
	// content-admin and plain manage nobody.
	if conversation.CanManageMember(content, plain) || conversation.CanManageMember(plain, plain) {
		t.Error("content-admin / plain manage nobody")
	}
	// the creator is untouchable by anyone else; and nobody manages themselves.
	if conversation.CanManageMember(super1, creatorAdmin) {
		t.Error("no one manages the creator")
	}
	if conversation.CanManageMember(super1, super1) {
		t.Error("no self-management")
	}
}

func TestCanAssignTier(t *testing.T) {
	self := &conversation.Member{UserID: uuid.Must(uuid.NewV7())}
	creatorSelf := &conversation.Member{UserID: self.UserID, IsCreator: true, AdminTier: conversation.TierNone}
	superA := &conversation.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: conversation.TierSuperAdmin}
	superB := &conversation.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: conversation.TierSuperAdmin}
	admin := &conversation.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: conversation.TierAdmin}
	plain := &conversation.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: conversation.TierNone}

	// creator self-restore.
	if !conversation.CanAssignTier(creatorSelf, creatorSelf, conversation.TierCreatorAdmin) {
		t.Error("creator may restore their own creator_admin")
	}
	// non-creator cannot take creator_admin, even for themselves.
	if conversation.CanAssignTier(superA, superA, conversation.TierCreatorAdmin) {
		t.Error("a non-creator may not self-assign creator_admin")
	}
	// voluntary self step-down.
	if !conversation.CanAssignTier(superA, superA, conversation.TierAdmin) {
		t.Error("a superadmin may step themselves down to admin")
	}
	// superadmin promotes a plain member to admin, not to superadmin.
	if !conversation.CanAssignTier(superA, plain, conversation.TierAdmin) {
		t.Error("superadmin may mint an admin")
	}
	if conversation.CanAssignTier(superA, plain, conversation.TierSuperAdmin) {
		t.Error("superadmin may not mint another superadmin")
	}
	// superadmin may not retier another superadmin.
	if conversation.CanAssignTier(superA, superB, conversation.TierAdmin) {
		t.Error("superadmin may not retier a peer superadmin")
	}
	// admin may not assign any tier.
	if conversation.CanAssignTier(admin, plain, conversation.TierContentAdmin) {
		t.Error("admin may not assign tiers")
	}
	// nobody assigns creator_admin to someone else.
	if conversation.CanAssignTier(creatorSelf, plain, conversation.TierCreatorAdmin) {
		t.Error("creator_admin is never assignable to another member")
	}
	// invalid tier value.
	if conversation.CanAssignTier(superA, plain, conversation.AdminTier("mod")) {
		t.Error("invalid tier must be rejected")
	}
	_ = self
}

func TestIsMuted(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name       string
		mutedUntil *time.Time
		want       bool
	}{
		{"nil is not muted", nil, false},
		{"future is muted", &future, true},
		{"past is not muted", &past, false},
		{"exactly now is not muted", &now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &conversation.Member{MutedUntil: tt.mutedUntil}
			if got := m.IsMuted(now); got != tt.want {
				t.Errorf("IsMuted() = %v, want %v", got, tt.want)
			}
		})
	}
}
