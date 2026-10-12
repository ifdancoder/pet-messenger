package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestSetArchived_CreatorArchives(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	pub := &fakePublisher{}
	h := command.NewSetArchivedHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	if err := h.Handle(context.Background(), in.SetArchivedCommand{Actor: creator, ConversationID: c.ID, Archived: true}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !c.IsArchived() {
		t.Fatalf("conversation not archived")
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (ConversationArchived)", len(pub.events))
	}
}

func TestSetArchived_NoOpWhenAlreadyInState(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	pub := &fakePublisher{}
	h := command.NewSetArchivedHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	if err := h.Handle(context.Background(), in.SetArchivedCommand{Actor: creator, ConversationID: c.ID, Archived: false}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatalf("published %d events for a no-op, want 0", len(pub.events))
	}
}
