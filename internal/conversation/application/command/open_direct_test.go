package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestOpenDirect_CreatesOnFirstCall(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := newFakeLoader()
	writer := &fakeWriter{loader: loader}
	pub := &fakePublisher{}
	users := &fakeUsers{exists: map[uuid.UUID]bool{other: true}}
	h := command.NewOpenDirectHandler(loader, writer, users, pub)

	c, err := h.Handle(context.Background(), in.OpenDirectCommand{Me: me, Other: other})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if c.Kind != domain.KindDirect {
		t.Fatalf("Kind = %v, want KindDirect", c.Kind)
	}
	if len(pub.events) != 3 { // ConversationCreated + 2x MemberAdded
		t.Fatalf("published %d events, want 3", len(pub.events))
	}
}

func TestOpenDirect_IdempotentOnSecondCall(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := newFakeLoader()
	writer := &fakeWriter{loader: loader}
	pub := &fakePublisher{}
	users := &fakeUsers{exists: map[uuid.UUID]bool{other: true}}
	h := command.NewOpenDirectHandler(loader, writer, users, pub)
	ctx := context.Background()

	first, err := h.Handle(ctx, in.OpenDirectCommand{Me: me, Other: other})
	if err != nil {
		t.Fatalf("first Handle() error = %v", err)
	}
	pub.events = nil // only the second call's publishes matter here

	second, err := h.Handle(ctx, in.OpenDirectCommand{Me: me, Other: other})
	if err != nil {
		t.Fatalf("second Handle() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second call returned a different conversation: %s != %s", second.ID, first.ID)
	}
	if len(pub.events) != 0 {
		t.Fatalf("second call published %d events, want 0", len(pub.events))
	}
}

func TestOpenDirect_RejectsSelf(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := command.NewOpenDirectHandler(newFakeLoader(), &fakeWriter{loader: newFakeLoader()}, &fakeUsers{}, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.OpenDirectCommand{Me: me, Other: me})
	if err != command.ErrCannotDirectSelf {
		t.Fatalf("err = %v, want ErrCannotDirectSelf", err)
	}
}

func TestOpenDirect_RecipientNotFound(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	h := command.NewOpenDirectHandler(newFakeLoader(), &fakeWriter{loader: newFakeLoader()}, &fakeUsers{}, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.OpenDirectCommand{Me: me, Other: other})
	if err != command.ErrRecipientNotFound {
		t.Fatalf("err = %v, want ErrRecipientNotFound", err)
	}
}
