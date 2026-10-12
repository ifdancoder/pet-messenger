package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestCreateChannel_Success(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	pub := &fakePublisher{}
	h := command.NewCreateChannelHandler(&fakeWriter{loader: newFakeLoader()}, pub)

	c, err := h.Handle(context.Background(), in.CreateChannelCommand{Creator: creator, Kind: domain.KindChannelBroadcast, Title: "News", Public: true})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if c.Kind != domain.KindChannelBroadcast || !c.Public {
		t.Fatalf("got Kind=%v Public=%v", c.Kind, c.Public)
	}
	if len(pub.events) != 2 {
		t.Fatalf("published %d events, want 2", len(pub.events))
	}
}

func TestCreateChannel_RejectsNonChannelKind(t *testing.T) {
	h := command.NewCreateChannelHandler(&fakeWriter{loader: newFakeLoader()}, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.CreateChannelCommand{Creator: uuid.Must(uuid.NewV7()), Kind: domain.KindGroupChat, Title: "x"})
	if err != domain.ErrInvalidKind {
		t.Fatalf("err = %v, want ErrInvalidKind", err)
	}
}
