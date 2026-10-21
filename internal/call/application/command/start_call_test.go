package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

func newStartCallHandler(calls *fakeCallStore, convs *fakeConversationReader, nodes *fakeMediaNodeRegistry, rooms *fakeRoomControl, parts *fakeParticipantStore, events *fakeSystemEventPoster, notif *fakeCallNotifier) in.StartCallUseCase {
	return NewStartCallHandler(calls, calls, convs, nodes, rooms, parts, fakeJoinTicketIssuer{}, fakeICEConfigProvider{}, events, notif)
}

func TestStartCall_HappyPath(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{node: out.Node{ID: "node-a", GRPCAddr: "grpc:1", WSAddr: "ws:2"}}
	rooms := &fakeRoomControl{}
	parts := newFakeParticipantStore()
	events := &fakeSystemEventPoster{}
	notif := &fakeCallNotifier{}
	h := newStartCallHandler(calls, convs, nodes, rooms, parts, events, notif)
	convID, memberID := groupChat(t, convs)

	c, ji, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(calls.created) != 1 {
		t.Fatalf("calls.created = %d, want 1", len(calls.created))
	}
	if len(rooms.created) != 1 {
		t.Fatalf("rooms.created = %d, want 1", len(rooms.created))
	}
	if got := parts.parts[c.ID]; len(got) != 1 || got[0].UserID != memberID {
		t.Fatalf("participant rows = %+v, want one for the actor", got)
	}
	if len(events.events) != 1 || events.events[0] != domainmsg.SystemCallStarted {
		t.Fatalf("events = %v, want [call_started]", events.events)
	}
	if notif.started != 1 {
		t.Fatalf("notif.started = %d, want 1", notif.started)
	}
	if ji.Ticket != "tkt-publisher" {
		t.Fatalf("ji.Ticket = %q, want tkt-publisher", ji.Ticket)
	}
	if ji.MediaWSURL != "ws://ws:2/rtc" {
		t.Fatalf("ji.MediaWSURL = %q, want ws://ws:2/rtc", ji.MediaWSURL)
	}
	if ji.Role != domain.RolePublisher {
		t.Fatalf("ji.Role = %q, want publisher", ji.Role)
	}
}

func TestStartCall_LiveCallExists(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{node: out.Node{ID: "node-a", GRPCAddr: "grpc:1", WSAddr: "ws:2"}}
	rooms := &fakeRoomControl{}
	parts := newFakeParticipantStore()
	events := &fakeSystemEventPoster{}
	notif := &fakeCallNotifier{}
	h := newStartCallHandler(calls, convs, nodes, rooms, parts, events, notif)
	convID, memberID := groupChat(t, convs)
	existing := mustCall(t, convID, addMember(convs, convID, conversation.TierNone))
	calls.seedLive(existing)

	got, ji, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != existing {
		t.Fatalf("Handle returned a different call")
	}
	if len(rooms.created) != 0 {
		t.Fatalf("rooms.created = %d, want 0", len(rooms.created))
	}
	if notif.started != 0 {
		t.Fatalf("notif.started = %d, want 0", notif.started)
	}
	if len(events.events) != 0 {
		t.Fatalf("events = %v, want none", events.events)
	}
	if ji.Ticket != "tkt-publisher" {
		t.Fatalf("ji.Ticket = %q", ji.Ticket)
	}
}

func TestStartCall_ChannelBroadcastNotSupported(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newStartCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, newFakeParticipantStore(), &fakeSystemEventPoster{}, &fakeCallNotifier{})
	convID := uuid.Must(uuid.NewV7())
	memberID := uuid.Must(uuid.NewV7())
	convs.conv = &conversation.Conversation{ID: convID, Kind: conversation.KindChannelBroadcast}
	convs.members[memberID] = &conversation.Member{ConversationID: convID, UserID: memberID}

	if _, _, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID}); !errors.Is(err, ErrCallNotSupported) {
		t.Fatalf("err = %v, want ErrCallNotSupported", err)
	}
}

func TestStartCall_ArchivedConversation(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newStartCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, newFakeParticipantStore(), &fakeSystemEventPoster{}, &fakeCallNotifier{})
	convID, memberID := groupChat(t, convs)
	now := time.Now().UTC()
	convs.conv.ArchivedAt = &now

	if _, _, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID}); !errors.Is(err, conversation.ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestStartCall_NonMember(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	h := newStartCallHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, newFakeParticipantStore(), &fakeSystemEventPoster{}, &fakeCallNotifier{})
	convID, _ := groupChat(t, convs)
	stranger := uuid.Must(uuid.NewV7())

	if _, _, err := h.Handle(context.Background(), in.StartCallCommand{Actor: stranger, ConversationID: convID}); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestStartCall_NoMediaNodes(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	nodes := &fakeMediaNodeRegistry{pickErr: ErrNoMediaNodes}
	h := newStartCallHandler(calls, convs, nodes, &fakeRoomControl{}, newFakeParticipantStore(), &fakeSystemEventPoster{}, &fakeCallNotifier{})
	convID, memberID := groupChat(t, convs)

	if _, _, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID}); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("err = %v, want ErrNoMediaNodes", err)
	}
}

func TestStartCall_RoomCreateFailsCompensates(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	rooms := &fakeRoomControl{createErr: errors.New("sfu down")}
	h := newStartCallHandler(calls, convs, &fakeMediaNodeRegistry{node: out.Node{ID: "node-a", GRPCAddr: "grpc:1", WSAddr: "ws:2"}}, rooms, newFakeParticipantStore(), &fakeSystemEventPoster{}, &fakeCallNotifier{})
	convID, memberID := groupChat(t, convs)

	if _, _, err := h.Handle(context.Background(), in.StartCallCommand{Actor: memberID, ConversationID: convID}); !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("err = %v, want ErrMediaUnavailable", err)
	}
	if len(calls.endedCalls) != 1 {
		t.Fatalf("calls.endedCalls = %v, want the compensated call", calls.endedCalls)
	}
	if len(calls.live) != 0 {
		t.Fatalf("a live call still remains: %v", calls.live)
	}
}
