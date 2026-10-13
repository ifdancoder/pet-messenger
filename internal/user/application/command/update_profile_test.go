// internal/user/application/command/update_profile_test.go
package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/user/application/command"
	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/domain"
)

func TestUpdateProfile_ChangesDisplayName(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(u))

	newName := "Alicia"
	got, err := h.Handle(context.Background(), in.UpdateProfileCommand{UserID: u.ID, DisplayName: &newName})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.DisplayName != "Alicia" {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, "Alicia")
	}
}

func TestUpdateProfile_RenamesUsername(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(u))

	newName := "alice_02"
	got, err := h.Handle(context.Background(), in.UpdateProfileCommand{UserID: u.ID, Username: &newName})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.Username != "alice_02" {
		t.Fatalf("Username = %q, want %q", got.Username, "alice_02")
	}
}

func TestUpdateProfile_BothFields(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(u))

	newDisplay, newName := "Alicia", "alice_02"
	got, err := h.Handle(context.Background(), in.UpdateProfileCommand{UserID: u.ID, DisplayName: &newDisplay, Username: &newName})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.DisplayName != "Alicia" || got.Username != "alice_02" {
		t.Fatalf("got DisplayName=%q Username=%q, want Alicia/alice_02", got.DisplayName, got.Username)
	}
}

func TestUpdateProfile_NoFields(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(u))

	_, err = h.Handle(context.Background(), in.UpdateProfileCommand{UserID: u.ID})
	if !errors.Is(err, command.ErrNoFields) {
		t.Fatalf("err = %v, want ErrNoFields", err)
	}
}

func TestUpdateProfile_InvalidDisplayName(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(u))

	empty := ""
	_, err = h.Handle(context.Background(), in.UpdateProfileCommand{UserID: u.ID, DisplayName: &empty})
	if !errors.Is(err, domain.ErrInvalidDisplayName) {
		t.Fatalf("err = %v, want ErrInvalidDisplayName", err)
	}
}

func TestUpdateProfile_UsernameTaken(t *testing.T) {
	alice, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	bob, err := domain.New("bob_01", "Bob")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := command.NewUpdateProfileHandler(newFakeUserStore(alice, bob))

	taken := "bob_01"
	_, err = h.Handle(context.Background(), in.UpdateProfileCommand{UserID: alice.ID, Username: &taken})
	if !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestUpdateProfile_UserNotFound(t *testing.T) {
	h := command.NewUpdateProfileHandler(newFakeUserStore())
	newName := "x"
	_, err := h.Handle(context.Background(), in.UpdateProfileCommand{UserID: uuid.Must(uuid.NewV7()), DisplayName: &newName})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
