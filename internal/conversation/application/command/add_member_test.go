package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

func TestAddMember_AdminCanAdd(t *testing.T) {
	admin, invitee := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(admin, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	users := &fakeUsers{exists: map[uuid.UUID]bool{invitee: true}}
	pub := &fakePublisher{}
	h := command.NewAddMemberHandler(loader, &fakeWriter{loader: loader}, users, pub, &fakeSystemEventPoster{})

	m, err := h.Handle(context.Background(), in.AddMemberCommand{Actor: admin, ConversationID: c.ID, Invitee: invitee})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if m.UserID != invitee {
		t.Fatalf("UserID = %s, want %s", m.UserID, invitee)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 (MemberAdded)", len(pub.events))
	}
}

func TestAddMember_PlainMemberForbidden(t *testing.T) {
	creator, plain, invitee := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	c, members, _ := domain.NewGroupChat(creator, "Team")
	loader := newFakeLoader()
	loader.put(c, members...)
	loader.put(c, &domain.Member{ConversationID: c.ID, UserID: plain, Role: domain.RoleMember})
	h := command.NewAddMemberHandler(loader, &fakeWriter{loader: loader}, &fakeUsers{}, &fakePublisher{}, &fakeSystemEventPoster{})

	_, err := h.Handle(context.Background(), in.AddMemberCommand{Actor: plain, ConversationID: c.ID, Invitee: invitee})
	if err != command.ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}
