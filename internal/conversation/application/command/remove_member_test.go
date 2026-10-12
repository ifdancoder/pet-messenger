package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestRemoveMember_CreatorKicksMember(t *testing.T) {
	creator, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: target, Role: domain.RoleMember})
	pub := &fakePublisher{}
	h := command.NewRemoveMemberHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	if err := h.Handle(context.Background(), in.RemoveMemberCommand{Actor: creator, ConversationID: c.ID, Target: target}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberRemoved)", len(pub.events))
	}
}

func TestRemoveMember_RejectsSelfKick(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	h := command.NewRemoveMemberHandler(newFakeLoader(), &fakeWriter{loader: newFakeLoader()}, &fakePublisher{}, &fakeSystemEventPoster{})

	err := h.Handle(context.Background(), in.RemoveMemberCommand{Actor: creator, ConversationID: uuid.Must(uuid.NewV7()), Target: creator})
	if err != command.ErrCannotKickSelf {
		t.Fatalf("err = %v, want ErrCannotKickSelf", err)
	}
}

func TestRemoveMember_PlainMemberForbidden(t *testing.T) {
	creator, plain, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: plain, Role: domain.RoleMember})
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: target, Role: domain.RoleMember})
	h := command.NewRemoveMemberHandler(loader, &fakeWriter{loader: loader}, &fakePublisher{}, &fakeSystemEventPoster{})

	err := h.Handle(context.Background(), in.RemoveMemberCommand{Actor: plain, ConversationID: c.ID, Target: target})
	if err != command.ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}
