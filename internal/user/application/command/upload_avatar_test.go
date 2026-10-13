// internal/user/application/command/upload_avatar_test.go
package command_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/user/application/command"
	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/domain"
)

func TestUploadAvatar_StoresAndUpdatesURL(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	store := newFakeUserStore(u)
	avatars := newFakeAvatarStore()
	h := command.NewUploadAvatarHandler(store, avatars)

	want := "/users/alice_01/avatar"
	data := []byte("fake-png-bytes")
	got, err := h.Handle(context.Background(), in.UploadAvatarCommand{
		UserID: u.ID, ContentType: "image/png", Data: bytes.NewReader(data), Size: int64(len(data)),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.AvatarURL != want {
		t.Fatalf("AvatarURL = %q, want %q", got.AvatarURL, want)
	}
	stored, ct, err := avatars.Get(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer stored.Close()
	if ct != "image/png" {
		t.Errorf("content type = %q, want image/png", ct)
	}
	persisted, err := store.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if persisted.AvatarURL != want {
		t.Fatalf("persisted AvatarURL = %q, want %q", persisted.AvatarURL, want)
	}
}

func TestUploadAvatar_UserNotFound(t *testing.T) {
	h := command.NewUploadAvatarHandler(newFakeUserStore(), newFakeAvatarStore())
	_, err := h.Handle(context.Background(), in.UploadAvatarCommand{UserID: uuid.Must(uuid.NewV7()), ContentType: "image/png", Data: bytes.NewReader(nil), Size: 0})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
