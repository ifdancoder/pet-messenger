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

type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
}
