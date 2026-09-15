package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/user"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		displayName string
		wantErr     bool
		wantDisplay string
	}{
		{"valid", "alice_01", "Alice", false, "Alice"},
		{"defaults display name to username", "bob", "", false, "bob"},
		{"too short", "ab", "", true, ""},
		{"too long", "this_username_is_way_too_long_okay", "", true, ""},
		{"invalid chars", "alice!", "", true, ""},
		{"display name too long", "alice_01", strings.Repeat("a", 65), true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := user.New(tt.username, tt.displayName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.Username != tt.username {
				t.Errorf("Username = %q, want %q", u.Username, tt.username)
			}
			if u.DisplayName != tt.wantDisplay {
				t.Errorf("DisplayName = %q, want %q", u.DisplayName, tt.wantDisplay)
			}
			if u.ID == uuid.Nil {
				t.Error("expected ID to be set")
			}
			if u.ID.Version() != 7 {
				t.Errorf("ID version = %d, want 7 (UUIDv7)", u.ID.Version())
			}
			if u.CreatedAt.IsZero() {
				t.Error("expected CreatedAt to be set")
			}
		})
	}
}

func TestSetDisplayName(t *testing.T) {
	u := &user.User{DisplayName: "Old"}
	if err := u.SetDisplayName("New Name"); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if u.DisplayName != "New Name" {
		t.Fatalf("DisplayName = %q, want %q", u.DisplayName, "New Name")
	}
	if err := u.SetDisplayName(""); !errors.Is(err, user.ErrInvalidDisplayName) {
		t.Fatalf("empty name: got %v, want ErrInvalidDisplayName", err)
	}
	if err := u.SetDisplayName(strings.Repeat("a", 65)); !errors.Is(err, user.ErrInvalidDisplayName) {
		t.Fatalf("65 chars: got %v, want ErrInvalidDisplayName", err)
	}
}

func TestRename(t *testing.T) {
	u := &user.User{Username: "old_name"}
	if err := u.Rename("new_name"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if u.Username != "new_name" {
		t.Fatalf("Username = %q, want %q", u.Username, "new_name")
	}
	if err := u.Rename("a"); !errors.Is(err, user.ErrInvalidUsername) {
		t.Fatalf("short name: got %v, want ErrInvalidUsername", err)
	}
}
