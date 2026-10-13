// internal/user/application/query/get_by_username_test.go
package query_test

import (
	"context"
	"errors"
	"testing"

	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/application/query"
	"pet-messenger/internal/user/domain"
)

func TestGetByUsername_Found(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := query.NewGetByUsernameHandler(newFakeUserStore(u))

	got, err := h.Handle(context.Background(), in.GetByUsernameQuery{Username: "alice_01"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("ID = %v, want %v", got.ID, u.ID)
	}
}

func TestGetByUsername_NotFound(t *testing.T) {
	h := query.NewGetByUsernameHandler(newFakeUserStore())
	_, err := h.Handle(context.Background(), in.GetByUsernameQuery{Username: "nobody"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
