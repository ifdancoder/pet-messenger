package user_test

import (
	"testing"

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
			if u.ID.String() == "" {
				t.Error("expected ID to be set")
			}
			if u.CreatedAt.IsZero() {
				t.Error("expected CreatedAt to be set")
			}
		})
	}
}
