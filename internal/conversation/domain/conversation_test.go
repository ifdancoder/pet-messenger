package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

func TestKindValid(t *testing.T) {
	tests := []struct {
		kind domain.Kind
		want bool
	}{
		{domain.KindDirect, true},
		{domain.KindGroupChat, true},
		{domain.KindChannelGroup, true},
		{domain.KindChannelBroadcast, true},
		{domain.Kind("bogus"), false},
		{domain.Kind(""), false},
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
		role domain.Role
		want bool
	}{
		{domain.RoleOwner, true},
		{domain.RoleAdmin, true},
		{domain.RoleMember, true},
		{domain.RoleSubscriber, true},
		{domain.Role("guest"), false},
		{domain.Role(""), false},
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

	c, members, err := domain.NewDirect(a, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Kind != domain.KindDirect {
		t.Errorf("Kind = %q, want %q", c.Kind, domain.KindDirect)
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
		if m.Role != domain.RoleMember {
			t.Errorf("member Role = %q, want %q", m.Role, domain.RoleMember)
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
			_, _, err := domain.NewDirect(tt.userA, tt.userB)
			if !errors.Is(err, domain.ErrInvalidDirectMembers) {
				t.Fatalf("err = %v, want ErrInvalidDirectMembers", err)
			}
		})
	}
}

func TestNewGroupChat(t *testing.T) {
	creator := uuid.New()

	c, members, err := domain.NewGroupChat(creator, "  Team Standup  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Kind != domain.KindGroupChat {
		t.Errorf("Kind = %q, want %q", c.Kind, domain.KindGroupChat)
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
	if members[0].Role != domain.RoleMember || !members[0].IsCreator || members[0].AdminTier != domain.TierCreatorAdmin {
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
			_, _, err := domain.NewGroupChat(creator, tt.title)
			if !errors.Is(err, domain.ErrInvalidTitle) {
				t.Fatalf("err = %v, want ErrInvalidTitle for %s title", err, tt.name)
			}
		})
	}
}

func TestNewGroupChatNilCreator(t *testing.T) {
	_, _, err := domain.NewGroupChat(uuid.Nil, "Team")
	if !errors.Is(err, domain.ErrInvalidCreator) {
		t.Fatalf("err = %v, want ErrInvalidCreator", err)
	}
}

func TestNewGroupChatTitleBoundary(t *testing.T) {
	creator := uuid.New()
	if _, _, err := domain.NewGroupChat(creator, strings.Repeat("x", 256)); err != nil {
		t.Fatalf("256-char title should be valid, got %v", err)
	}
}

func TestConstantValues(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"KindDirect", string(domain.KindDirect), "direct"},
		{"KindGroupChat", string(domain.KindGroupChat), "group_chat"},
		{"KindChannelGroup", string(domain.KindChannelGroup), "channel_group"},
		{"KindChannelBroadcast", string(domain.KindChannelBroadcast), "channel_broadcast"},
		{"RoleOwner", string(domain.RoleOwner), "owner"},
		{"RoleAdmin", string(domain.RoleAdmin), "admin"},
		{"RoleMember", string(domain.RoleMember), "member"},
		{"RoleSubscriber", string(domain.RoleSubscriber), "subscriber"},
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
		kind        domain.Kind
		member      *domain.Member
		wantCanPost bool
	}{
		{domain.KindDirect, &domain.Member{Role: domain.RoleMember}, true},
		{domain.KindDirect, &domain.Member{Role: domain.RoleSubscriber}, false},
		{domain.KindGroupChat, &domain.Member{Role: domain.RoleMember}, true},
		{domain.KindGroupChat, &domain.Member{Role: domain.RoleSubscriber}, false},
		{domain.KindChannelGroup, &domain.Member{Role: domain.RoleMember}, true},
		{domain.KindChannelGroup, &domain.Member{Role: domain.RoleSubscriber}, false},
		{domain.KindChannelBroadcast, &domain.Member{Role: domain.RoleMember}, false},
		{domain.KindChannelBroadcast, &domain.Member{Role: domain.RoleSubscriber}, false},
		{domain.KindChannelBroadcast, &domain.Member{Role: domain.RoleMember, IsCreator: true}, true},
		{domain.KindChannelBroadcast, &domain.Member{Role: domain.RoleMember, AdminTier: domain.TierContentAdmin}, true},
		{domain.KindChannelBroadcast, &domain.Member{Role: domain.RoleMember, AdminTier: domain.TierSuperAdmin}, true},
	}
	for _, tc := range cases {
		c := &domain.Conversation{Kind: tc.kind}
		if got := c.CanPost(tc.member); got != tc.wantCanPost {
			t.Errorf("Conversation{Kind:%q}.CanPost(%+v) = %v, want %v", tc.kind, tc.member, got, tc.wantCanPost)
		}
	}
	if (&domain.Conversation{Kind: domain.Kind("bogus")}).CanPost(&domain.Member{}) {
		t.Error("CanPost on unknown kind should be false")
	}
}

func TestNewChannel(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	for _, kind := range []domain.Kind{domain.KindChannelGroup, domain.KindChannelBroadcast} {
		c, members, err := domain.NewChannel(creator, kind, "  Announcements  ", true)
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
			members[0].Role != domain.RoleMember || !members[0].IsCreator ||
			members[0].AdminTier != domain.TierCreatorAdmin {
			t.Errorf("members = %+v", members)
		}
	}
}

func TestNewChannel_PrivateDefault(t *testing.T) {
	c, _, err := domain.NewChannel(uuid.Must(uuid.NewV7()), domain.KindChannelGroup, "x", false)
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
		kind    domain.Kind
		title   string
		wantErr error
	}{
		{"nil creator", uuid.Nil, domain.KindChannelGroup, "x", domain.ErrInvalidCreator},
		{"kind direct", creator, domain.KindDirect, "x", domain.ErrInvalidKind},
		{"kind group_chat", creator, domain.KindGroupChat, "x", domain.ErrInvalidKind},
		{"blank title", creator, domain.KindChannelBroadcast, "   ", domain.ErrInvalidTitle},
		{"title too long", creator, domain.KindChannelGroup, strings.Repeat("x", 257), domain.ErrInvalidTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := domain.NewChannel(tt.creator, tt.kind, tt.title, false)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestKind_IsChannel(t *testing.T) {
	for k, want := range map[domain.Kind]bool{
		domain.KindDirect:           false,
		domain.KindGroupChat:        false,
		domain.KindChannelGroup:     true,
		domain.KindChannelBroadcast: true,
		domain.Kind("bogus"):        false,
	} {
		if got := k.IsChannel(); got != want {
			t.Errorf("Kind(%q).IsChannel() = %v, want %v", k, got, want)
		}
	}
}

func TestAdminTier_ValidAndRankOrder(t *testing.T) {
	for _, ok := range []domain.AdminTier{
		domain.TierCreatorAdmin, domain.TierSuperAdmin,
		domain.TierAdmin, domain.TierContentAdmin, domain.TierNone,
	} {
		if !ok.Valid() {
			t.Errorf("%q should be Valid", ok)
		}
	}
	if domain.AdminTier("mod").Valid() {
		t.Error(`AdminTier("mod") should be invalid`)
	}
}

func TestCanModerateMessages(t *testing.T) {
	for tier, want := range map[domain.AdminTier]bool{
		domain.TierNone:         false,
		domain.TierContentAdmin: true,
		domain.TierAdmin:        true,
		domain.TierSuperAdmin:   true,
		domain.TierCreatorAdmin: true,
	} {
		m := &domain.Member{AdminTier: tier}
		if got := m.CanModerateMessages(); got != want {
			t.Errorf("tier %q: CanModerateMessages() = %v, want %v", tier, got, want)
		}
	}
}

func TestCanAddMembers(t *testing.T) {
	admin := &domain.Member{AdminTier: domain.TierAdmin}
	plain := &domain.Member{AdminTier: domain.TierNone}
	content := &domain.Member{AdminTier: domain.TierContentAdmin}
	if (&domain.Conversation{Kind: domain.KindDirect}).CanAddMembers(admin) {
		t.Error("no one may add members to a direct conversation")
	}
	gc := &domain.Conversation{Kind: domain.KindGroupChat}
	if !gc.CanAddMembers(admin) {
		t.Error("an admin may add members to a group chat")
	}
	if gc.CanAddMembers(plain) || gc.CanAddMembers(content) {
		t.Error("a plain member / content-admin may not add members")
	}
}

func TestCanManageMember(t *testing.T) {
	mk := func(tier domain.AdminTier, creator bool) *domain.Member {
		return &domain.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: tier, IsCreator: creator}
	}
	creatorAdmin := mk(domain.TierCreatorAdmin, true)
	super1 := mk(domain.TierSuperAdmin, false)
	super2 := mk(domain.TierSuperAdmin, false)
	admin := mk(domain.TierAdmin, false)
	content := mk(domain.TierContentAdmin, false)
	plain := mk(domain.TierNone, false)

	// creator_admin manages everyone below.
	for _, target := range []*domain.Member{super1, admin, content, plain} {
		if !domain.CanManageMember(creatorAdmin, target) {
			t.Errorf("creator_admin should manage %q", target.AdminTier)
		}
	}
	// superadmin manages strictly lower, not another superadmin.
	if !domain.CanManageMember(super1, admin) || !domain.CanManageMember(super1, content) || !domain.CanManageMember(super1, plain) {
		t.Error("superadmin should manage admin/content/plain")
	}
	if domain.CanManageMember(super1, super2) {
		t.Error("superadmin must not manage another superadmin")
	}
	// admin manages only plain members.
	if !domain.CanManageMember(admin, plain) {
		t.Error("admin should manage a plain member")
	}
	if domain.CanManageMember(admin, content) || domain.CanManageMember(admin, super1) {
		t.Error("admin must not manage any tier holder")
	}
	// content-admin and plain manage nobody.
	if domain.CanManageMember(content, plain) || domain.CanManageMember(plain, plain) {
		t.Error("content-admin / plain manage nobody")
	}
	// the creator is untouchable by anyone else; and nobody manages themselves.
	if domain.CanManageMember(super1, creatorAdmin) {
		t.Error("no one manages the creator")
	}
	if domain.CanManageMember(super1, super1) {
		t.Error("no self-management")
	}
}

func TestCanAssignTier(t *testing.T) {
	self := &domain.Member{UserID: uuid.Must(uuid.NewV7())}
	creatorSelf := &domain.Member{UserID: self.UserID, IsCreator: true, AdminTier: domain.TierNone}
	creatorAdmin := &domain.Member{UserID: uuid.Must(uuid.NewV7()), IsCreator: true, AdminTier: domain.TierCreatorAdmin}
	superA := &domain.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: domain.TierSuperAdmin}
	superB := &domain.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: domain.TierSuperAdmin}
	admin := &domain.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: domain.TierAdmin}
	plain := &domain.Member{UserID: uuid.Must(uuid.NewV7()), AdminTier: domain.TierNone}

	// creator self-restore.
	if !domain.CanAssignTier(creatorSelf, creatorSelf, domain.TierCreatorAdmin) {
		t.Error("creator may restore their own creator_admin")
	}
	// a real creator_admin-tier actor is the only path that mints a superadmin.
	if !domain.CanAssignTier(creatorAdmin, plain, domain.TierSuperAdmin) {
		t.Error("creator_admin may mint a superadmin")
	}
	// creator_admin may retier a superadmin, being strictly higher in rank.
	if !domain.CanAssignTier(creatorAdmin, superB, domain.TierAdmin) {
		t.Error("creator_admin may retier a superadmin")
	}
	// non-creator cannot take creator_admin, even for themselves.
	if domain.CanAssignTier(superA, superA, domain.TierCreatorAdmin) {
		t.Error("a non-creator may not self-assign creator_admin")
	}
	// voluntary self step-down.
	if !domain.CanAssignTier(superA, superA, domain.TierAdmin) {
		t.Error("a superadmin may step themselves down to admin")
	}
	// superadmin promotes a plain member to admin, not to superadmin.
	if !domain.CanAssignTier(superA, plain, domain.TierAdmin) {
		t.Error("superadmin may mint an admin")
	}
	if domain.CanAssignTier(superA, plain, domain.TierSuperAdmin) {
		t.Error("superadmin may not mint another superadmin")
	}
	// superadmin may not retier another superadmin.
	if domain.CanAssignTier(superA, superB, domain.TierAdmin) {
		t.Error("superadmin may not retier a peer superadmin")
	}
	// admin may not assign any tier.
	if domain.CanAssignTier(admin, plain, domain.TierContentAdmin) {
		t.Error("admin may not assign tiers")
	}
	// nobody assigns creator_admin to someone else: tested against a real
	// creator_admin-tier actor so this exercises the TierCreatorAdmin guard,
	// not the rank gate.
	if domain.CanAssignTier(creatorAdmin, plain, domain.TierCreatorAdmin) {
		t.Error("creator_admin is never assignable to another member")
	}
	// invalid tier value.
	if domain.CanAssignTier(superA, plain, domain.AdminTier("mod")) {
		t.Error("invalid tier must be rejected")
	}
}

func TestIsArchived(t *testing.T) {
	if (&domain.Conversation{}).IsArchived() {
		t.Error("a conversation with nil ArchivedAt is not archived")
	}
	now := time.Now()
	if !(&domain.Conversation{ArchivedAt: &now}).IsArchived() {
		t.Error("a conversation with a set ArchivedAt is archived")
	}
}

func TestCanArchive(t *testing.T) {
	for tier, want := range map[domain.AdminTier]bool{
		domain.TierNone:         false,
		domain.TierContentAdmin: false,
		domain.TierAdmin:        false,
		domain.TierSuperAdmin:   true,
		domain.TierCreatorAdmin: true,
	} {
		if got := domain.CanArchive(&domain.Member{AdminTier: tier}); got != want {
			t.Errorf("CanArchive(tier %q) = %v, want %v", tier, got, want)
		}
	}
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
			m := &domain.Member{MutedUntil: tt.mutedUntil}
			if got := m.IsMuted(now); got != tt.want {
				t.Errorf("IsMuted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdminTier_AtLeast(t *testing.T) {
	cases := []struct {
		t, other domain.AdminTier
		want     bool
	}{
		{domain.TierAdmin, domain.TierAdmin, true},
		{domain.TierSuperAdmin, domain.TierAdmin, true},
		{domain.TierCreatorAdmin, domain.TierSuperAdmin, true},
		{domain.TierContentAdmin, domain.TierAdmin, false},
		{domain.TierNone, domain.TierContentAdmin, false},
		{domain.TierAdmin, domain.TierNone, true},
	}
	for _, c := range cases {
		if got := c.t.AtLeast(c.other); got != c.want {
			t.Errorf("%q.AtLeast(%q) = %v, want %v", c.t, c.other, got, c.want)
		}
	}
}
