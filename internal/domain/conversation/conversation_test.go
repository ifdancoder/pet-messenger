package conversation_test

import (
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
	if c.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
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
		seen[m.UserID] = true
	}
	if !seen[a] || !seen[b] {
		t.Errorf("members %+v do not cover both users %v, %v", members, a, b)
	}
}

func TestNewDirectSameUser(t *testing.T) {
	a := uuid.New()
	if _, _, err := conversation.NewDirect(a, a); err == nil {
		t.Fatal("expected error for identical users, got nil")
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
	if members[0].Role != conversation.RoleOwner {
		t.Errorf("member Role = %q, want %q", members[0].Role, conversation.RoleOwner)
	}
	if members[0].ConversationID != c.ID {
		t.Errorf("member ConversationID = %v, want %v", members[0].ConversationID, c.ID)
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
			if _, _, err := conversation.NewGroupChat(creator, tt.title); err == nil {
				t.Fatalf("expected error for %s title, got nil", tt.name)
			}
		})
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
	roles := []conversation.Role{
		conversation.RoleOwner,
		conversation.RoleAdmin,
		conversation.RoleMember,
		conversation.RoleSubscriber,
	}
	// want[kind][role] = expected CanPost result
	want := map[conversation.Kind]map[conversation.Role]bool{
		conversation.KindDirect: {
			conversation.RoleOwner: true, conversation.RoleAdmin: true,
			conversation.RoleMember: true, conversation.RoleSubscriber: false,
		},
		conversation.KindGroupChat: {
			conversation.RoleOwner: true, conversation.RoleAdmin: true,
			conversation.RoleMember: true, conversation.RoleSubscriber: false,
		},
		conversation.KindChannelGroup: {
			conversation.RoleOwner: true, conversation.RoleAdmin: true,
			conversation.RoleMember: true, conversation.RoleSubscriber: false,
		},
		conversation.KindChannelBroadcast: {
			conversation.RoleOwner: true, conversation.RoleAdmin: true,
			conversation.RoleMember: false, conversation.RoleSubscriber: false,
		},
	}

	for kind, roleWants := range want {
		for _, role := range roles {
			c := &conversation.Conversation{Kind: kind}
			if got := c.CanPost(role); got != roleWants[role] {
				t.Errorf("Conversation{Kind:%q}.CanPost(%q) = %v, want %v", kind, role, got, roleWants[role])
			}
		}
	}

	if (&conversation.Conversation{Kind: conversation.Kind("bogus")}).CanPost(conversation.RoleOwner) {
		t.Error("CanPost on unknown kind should be false")
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
			m := &conversation.Member{MutedUntil: tt.mutedUntil}
			if got := m.IsMuted(now); got != tt.want {
				t.Errorf("IsMuted() = %v, want %v", got, tt.want)
			}
		})
	}
}
