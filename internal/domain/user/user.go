package user

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidUsername    = errors.New("user: invalid username")
	ErrUsernameTaken      = errors.New("user: username already taken")
	ErrNotFound           = errors.New("user: not found")
	ErrInvalidDisplayName = errors.New("user: display name must be at most 64 characters")
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

type User struct {
	ID           uuid.UUID
	Username     string
	DisplayName  string
	AvatarURL    string
	PasswordHash string
	CreatedAt    time.Time
}

// New constructs a User with a fresh ID and validated username.
// PasswordHash must be set separately once the caller has hashed the password.
func New(username, displayName string) (*User, error) {
	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 64 {
		return nil, ErrInvalidDisplayName
	}
	return &User{
		ID:          uuid.Must(uuid.NewV7()),
		Username:    username,
		DisplayName: displayName,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// SetDisplayName validates and applies a display-name change in place —
// same rule New enforces at creation, minus New's "default to username
// when empty" fallback: an explicit profile edit clearing the field is a
// mistake to reject, not a value to silently substitute.
func (u *User) SetDisplayName(displayName string) error {
	if displayName == "" || len(displayName) > 64 {
		return ErrInvalidDisplayName
	}
	u.DisplayName = displayName
	return nil
}

// Rename validates and applies a username change in place. The repository
// (not this method) is what surfaces a collision, via ErrUsernameTaken —
// same division of responsibility as Create.
func (u *User) Rename(username string) error {
	if !usernamePattern.MatchString(username) {
		return ErrInvalidUsername
	}
	u.Username = username
	return nil
}

type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)

	// Update persists u's current Username, DisplayName, and AvatarURL.
	// Returns ErrUsernameTaken on a collision, ErrNotFound if u.ID doesn't
	// exist.
	Update(ctx context.Context, u *User) error
}
