package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestLeaveConversation_RemovesMembership(t *testing.T) {
	creator, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: other, Role: domain.RoleMember})
	pub := &fakePublisher{}
	h := command.NewLeaveConversationHandler(loader, &fakeWriter{loader: loader}, pub, &fakeSystemEventPoster{})

	if err := h.Handle(context.Background(), in.LeaveConversationCommand{Actor: other, ConversationID: c.ID}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := loader.GetMember(context.Background(), c.ID, other); err != domain.ErrNotMember {
		t.Fatalf("GetMember after leave = %v, want ErrNotMember", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberRemoved)", len(pub.events))
	}
}

func TestLeaveConversation_RejectsDirect(t *testing.T) {
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewDirect(a, b)
	loader := newFakeLoader()
	loader.put(c, members...)
	h := command.NewLeaveConversationHandler(loader, &fakeWriter{loader: loader}, &fakePublisher{}, &fakeSystemEventPoster{})

	err := h.Handle(context.Background(), in.LeaveConversationCommand{Actor: a, ConversationID: c.ID})
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
