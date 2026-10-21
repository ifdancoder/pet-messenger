package command

import (
	"context"
	"errors"
	"testing"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
)

func newJoinCallHandler(calls *fakeCallStore, convs *fakeConversationReader, nodes *fakeMediaNodeRegistry, parts *fakeParticipantStore) in.JoinCallUseCase {
	return NewJoinCallHandler(calls, calls, convs, nodes, parts, fakeJoinTicketIssuer{}, fakeICEConfigProvider{})
}

func TestJoinCall_HappyPath(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{node: out.Node{ID: "node-a"}}
	parts := newFakeParticipantStore()
	h := newJoinCallHandler(calls, convs, nodes, parts)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)
	joinerID := addMember(convs, convID, conversation.TierNone)

	ji, err := h.Handle(context.Background(), in.JoinCallCommand{Actor: joinerID, CallID: c.ID, Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if ji.Role != domain.RoleViewer {
		t.Fatalf("ji.Role = %q, want viewer", ji.Role)
	}
	var found bool
	for _, p := range parts.parts[c.ID] {
		if p.UserID == joinerID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no participant row added for the joiner")
	}
}

func TestJoinCall_NotLive(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newJoinCallHandler(calls, convs, &fakeMediaNodeRegistry{}, newFakeParticipantStore())
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	_ = c.End()
	calls.byID[c.ID] = c

	if _, err := h.Handle(context.Background(), in.JoinCallCommand{Actor: initiatorID, CallID: c.ID, Role: domain.RolePublisher}); !errors.Is(err, domain.ErrCallNotLive) {
		t.Fatalf("err = %v, want ErrCallNotLive", err)
	}
}

func TestJoinCall_AlreadyParticipantReconnects(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{node: out.Node{ID: "node-a"}}
	parts := newFakeParticipantStore()
	parts.addPartErr = domain.ErrAlreadyInCall
	h := newJoinCallHandler(calls, convs, nodes, parts)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)

	ji, err := h.Handle(context.Background(), in.JoinCallCommand{Actor: memberID, CallID: c.ID, Role: domain.RolePublisher})
	if err != nil {
		t.Fatalf("Handle: %v, want nil (reconnect)", err)
	}
	if ji.Ticket == "" {
		t.Fatalf("expected a JoinInfo with a ticket on reconnect")
	}
}

func TestJoinCall_MediaNodeGone(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{getErr: ErrNoMediaNodes}
	h := newJoinCallHandler(calls, convs, nodes, newFakeParticipantStore())
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)

	if _, err := h.Handle(context.Background(), in.JoinCallCommand{Actor: memberID, CallID: c.ID, Role: domain.RolePublisher}); !errors.Is(err, ErrMediaNodeGone) {
		t.Fatalf("err = %v, want ErrMediaNodeGone", err)
	}
}
