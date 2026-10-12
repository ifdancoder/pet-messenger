package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestClaimCreator_HeirClaims(t *testing.T) {
	heir := uuid.Must(uuid.NewV7())
	c := &domain.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domain.KindGroupChat, Title: "Team"}
	loader := newFakeLoader()
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: heir, Role: domain.RoleMember})
	pub := &fakePublisher{}
	h := command.NewClaimCreatorHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	m, err := h.Handle(context.Background(), in.ClaimCreatorCommand{Actor: heir, ConversationID: c.ID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !m.IsCreator || m.AdminTier != domain.TierCreatorAdmin {
		t.Fatalf("got IsCreator=%v AdminTier=%v", m.IsCreator, m.AdminTier)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberTierChanged)", len(pub.events))
	}
}

func TestClaimCreator_RejectsDirect(t *testing.T) {
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewDirect(a, b)
	loader := newFakeLoader()
	loader.put(c, members...)
	h := command.NewClaimCreatorHandler(loader, &fakeWriter{loader: loader}, &fakePublisher{}, &fakeSystemEventPoster{})

	_, err := h.Handle(context.Background(), in.ClaimCreatorCommand{Actor: a, ConversationID: c.ID})
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
