package objectstore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/infrastructure/objectstore"
	userout "pet-messenger/internal/user/application/port/out"
)

func TestMinioAvatarStore_PutGetRoundTrip(t *testing.T) {
	cfg := startMinioContainer(t)
	client, err := objectstore.NewClient(fxNoopLifecycle{}, cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store := objectstore.NewMinioAvatarStore(client, cfg)

	userID := uuid.Must(uuid.NewV7())
	payload := []byte("fake-png-bytes")
	if err := store.Put(context.Background(), userID, "image/png", bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, contentType, err := store.Get(context.Background(), userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) || contentType != "image/png" {
		t.Fatalf("got %q/%q, want %q/image/png", got, contentType, payload)
	}
}

func TestMinioAvatarStore_GetMissing(t *testing.T) {
	cfg := startMinioContainer(t)
	client, err := objectstore.NewClient(fxNoopLifecycle{}, cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store := objectstore.NewMinioAvatarStore(client, cfg)
	_, _, err = store.Get(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, userout.ErrAvatarNotFound) {
		t.Fatalf("got %v, want ErrAvatarNotFound", err)
	}
}
