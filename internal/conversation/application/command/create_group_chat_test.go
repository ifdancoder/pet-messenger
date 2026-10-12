package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestCreateGroupChat_Success(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	loader := newFakeLoader()
	pub := &fakePublisher{}
	h := command.NewCreateGroupChatHandler(&fakeWriter{loader: loader}, pub)

	c, err := h.Handle(context.Background(), in.CreateGroupChatCommand{Creator: creator, Title: "Team"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if c.Kind != domain.KindGroupChat || c.Title != "Team" {
		t.Fatalf("got Kind=%v Title=%q", c.Kind, c.Title)
	}
	if len(pub.events) != 2 { // ConversationCreated + MemberAdded
		t.Fatalf("published %d events, want 2", len(pub.events))
	}
}

func TestCreateGroupChat_InvalidTitle(t *testing.T) {
	h := command.NewCreateGroupChatHandler(&fakeWriter{loader: newFakeLoader()}, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.CreateGroupChatCommand{Creator: uuid.Must(uuid.NewV7()), Title: "  "})
	if err != domain.ErrInvalidTitle {
		t.Fatalf("err = %v, want ErrInvalidTitle", err)
	}
}
