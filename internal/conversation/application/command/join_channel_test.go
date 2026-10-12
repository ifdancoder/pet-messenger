package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestJoinChannel_JoinsPublicChannel(t *testing.T) {
	creator, joiner := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, err := domain.NewChannel(creator, domain.KindChannelGroup, "Public", true)
	if err != nil {
		t.Fatalf("NewChannel() error = %v", err)
	}
	loader := newFakeLoader()
	loader.put(c, members...)
	pub := &fakePublisher{}
	poster := &fakeSystemEventPoster{}
	h := command.NewJoinChannelHandler(loader, &fakeWriter{loader: loader}, pub, poster)

	got, err := h.Handle(context.Background(), in.JoinChannelCommand{Actor: joiner, ChannelID: c.ID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got.ID != c.ID {
		t.Fatalf("got conversation %s, want %s", got.ID, c.ID)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberAdded)", len(pub.events))
	}
	if poster.posts != 1 {
		t.Fatalf("posted %d system events, want 1", poster.posts)
	}
}

func TestJoinChannel_IdempotentForExistingMember(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewChannel(creator, domain.KindChannelGroup, "Public", true)
	loader := newFakeLoader()
	loader.put(c, members...)
	pub := &fakePublisher{}
	poster := &fakeSystemEventPoster{}
	h := command.NewJoinChannelHandler(loader, &fakeWriter{loader: loader}, pub, poster)

	_, err := h.Handle(context.Background(), in.JoinChannelCommand{Actor: creator, ChannelID: c.ID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(pub.events) != 0 || poster.posts != 0 {
		t.Fatalf("idempotent join published %d events / %d posts, want 0/0", len(pub.events), poster.posts)
	}
}

func TestJoinChannel_PrivateChannelNotFound(t *testing.T) {
	creator, joiner := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewChannel(creator, domain.KindChannelGroup, "Private", false)
	loader := newFakeLoader()
	loader.put(c, members...)
	h := command.NewJoinChannelHandler(loader, &fakeWriter{loader: loader}, &fakePublisher{}, &fakeSystemEventPoster{})

	_, err := h.Handle(context.Background(), in.JoinChannelCommand{Actor: joiner, ChannelID: c.ID})
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
