// internal/user/application/query/serve_avatar_test.go
package query_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/port/out"
	"pet-messenger/internal/user/application/query"
	"pet-messenger/internal/user/domain"
)

func TestServeAvatar_StreamsStoredBytes(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	u.AvatarURL = "/users/alice_01/avatar"
	avatars := newFakeAvatarStore()
	avatars.put(u.ID, "image/png", []byte("png-bytes"))
	h := query.NewServeAvatarHandler(newFakeUserStore(u), avatars)

	data, ct, err := h.Handle(context.Background(), in.ServeAvatarQuery{Username: "alice_01"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	defer data.Close()
	if ct != "image/png" {
		t.Errorf("content type = %q, want image/png", ct)
	}
	got, err := io.ReadAll(data)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	if !bytes.Equal(got, []byte("png-bytes")) {
		t.Errorf("bytes = %q, want \"png-bytes\"", got)
	}
}

func TestServeAvatar_NeverUploaded(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := query.NewServeAvatarHandler(newFakeUserStore(u), newFakeAvatarStore())

	_, _, err = h.Handle(context.Background(), in.ServeAvatarQuery{Username: "alice_01"})
	if !errors.Is(err, out.ErrAvatarNotFound) {
		t.Fatalf("err = %v, want ErrAvatarNotFound", err)
	}
}

func TestServeAvatar_UnknownUsername(t *testing.T) {
	h := query.NewServeAvatarHandler(newFakeUserStore(), newFakeAvatarStore())
	_, _, err := h.Handle(context.Background(), in.ServeAvatarQuery{Username: "nobody"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
