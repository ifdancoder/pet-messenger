package conversation_test

import (
	"strings"
	"testing"

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
