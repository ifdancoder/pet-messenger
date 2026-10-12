package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestSetMemberTier_CreatorPromotesMember(t *testing.T) {
	creator, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: target, Role: domain.RoleMember})
	pub := &fakePublisher{}
	h := command.NewSetMemberTierHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	m, err := h.Handle(context.Background(), in.SetMemberTierCommand{Actor: creator, ConversationID: c.ID, Target: target, Tier: domain.TierAdmin})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if m.AdminTier != domain.TierAdmin {
		t.Fatalf("AdminTier = %v, want TierAdmin", m.AdminTier)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberTierChanged)", len(pub.events))
	}
}

func TestSetMemberTier_RejectsUnknownTier(t *testing.T) {
	h := command.NewSetMemberTierHandler(newFakeLoader(), &fakeWriter{loader: newFakeLoader()}, &fakePublisher{}, &fakeSystemEventPoster{})

	_, err := h.Handle(context.Background(), in.SetMemberTierCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), Target: uuid.Must(uuid.NewV7()), Tier: domain.AdminTier("bogus")})
	if err != command.ErrInvalidTier {
		t.Fatalf("err = %v, want ErrInvalidTier", err)
	}
}
