package query

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

type fakeConversationReader struct {
	conv    *conversation.Conversation
	members map[uuid.UUID]*conversation.Member
}

func newFakeConversationReader() *fakeConversationReader {
	return &fakeConversationReader{members: map[uuid.UUID]*conversation.Member{}}
}

func (f *fakeConversationReader) GetByID(_ context.Context, id uuid.UUID) (*conversation.Conversation, error) {
	if f.conv != nil && f.conv.ID == id {
		return f.conv, nil
	}
	return nil, conversation.ErrNotFound
}

func (f *fakeConversationReader) GetMember(_ context.Context, _, userID uuid.UUID) (*conversation.Member, error) {
	if m, ok := f.members[userID]; ok {
		return m, nil
	}
	return nil, conversation.ErrNotMember
}

type fakeCallStore struct {
	live       map[uuid.UUID]*domain.Call
	byID       map[uuid.UUID]*domain.Call
	endedCalls []uuid.UUID
}

func newFakeCallStore() *fakeCallStore {
	return &fakeCallStore{live: map[uuid.UUID]*domain.Call{}, byID: map[uuid.UUID]*domain.Call{}}
}

func (f *fakeCallStore) Create(_ context.Context, c *domain.Call) error {
	f.byID[c.ID] = c
	f.live[c.ConversationID] = c
	return nil
}

func (f *fakeCallStore) MarkEnded(_ context.Context, id uuid.UUID, _ time.Time) (bool, error) {
	c, ok := f.byID[id]
	if !ok || !c.IsLive() {
		return false, nil
	}
	_ = c.End()
	delete(f.live, c.ConversationID)
	f.endedCalls = append(f.endedCalls, id)
	return true, nil
}

func (f *fakeCallStore) GetByID(_ context.Context, id uuid.UUID) (*domain.Call, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeCallStore) GetLiveForConversation(_ context.Context, convID uuid.UUID) (*domain.Call, error) {
	if c, ok := f.live[convID]; ok && c.IsLive() {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeCallStore) ListLive(context.Context) ([]*domain.Call, error) { return nil, nil }

func (f *fakeCallStore) seedLive(c *domain.Call) {
	f.byID[c.ID] = c
	f.live[c.ConversationID] = c
}

type fakeMediaNodeRegistry struct {
	node   out.Node
	getErr error
}

func (f *fakeMediaNodeRegistry) PickLeastLoaded(context.Context) (out.Node, error) {
	return out.Node{}, nil
}
func (f *fakeMediaNodeRegistry) Get(context.Context, string) (out.Node, error) {
	if f.getErr != nil {
		return out.Node{}, f.getErr
	}
	return f.node, nil
}

type fakeRoomControl struct {
	listParticipants    []out.ParticipantInfo
	listParticipantsErr error
}

func (fakeRoomControl) CreateRoom(context.Context, out.Node, uuid.UUID) error { return nil }
func (fakeRoomControl) CloseRoom(context.Context, out.Node, uuid.UUID) error  { return nil }
func (fakeRoomControl) MuteParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (fakeRoomControl) UnmuteParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (fakeRoomControl) RemoveParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeRoomControl) ListParticipants(context.Context, out.Node, uuid.UUID) ([]out.ParticipantInfo, error) {
	if f.listParticipantsErr != nil {
		return nil, f.listParticipantsErr
	}
	return f.listParticipants, nil
}

type fakeParticipantLister struct {
	history []*domain.Participant
}

func (f *fakeParticipantLister) ListParticipants(context.Context, uuid.UUID) ([]*domain.Participant, error) {
	return f.history, nil
}

type fakeSystemEventPoster struct{ events []domainmsg.SystemEvent }

func (f *fakeSystemEventPoster) PostSystemEvent(_ context.Context, _ *conversation.Conversation, _ *uuid.UUID, e domainmsg.SystemEvent, _ uuid.UUID, _ string) error {
	f.events = append(f.events, e)
	return nil
}

func groupChat(t *testing.T, convs *fakeConversationReader) (convID, memberID uuid.UUID) {
	t.Helper()
	convID = uuid.Must(uuid.NewV7())
	memberID = uuid.Must(uuid.NewV7())
	convs.conv = &conversation.Conversation{ID: convID, Kind: conversation.KindGroupChat}
	convs.members[memberID] = &conversation.Member{ConversationID: convID, UserID: memberID, Role: conversation.RoleMember}
	return
}

func mustCall(t *testing.T, convID, initiatorID uuid.UUID) *domain.Call {
	t.Helper()
	c, err := domain.New(convID, initiatorID, "node-a")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	return c
}

func newHandler(calls *fakeCallStore, convs *fakeConversationReader, nodes *fakeMediaNodeRegistry, rooms *fakeRoomControl, parts *fakeParticipantLister, events *fakeSystemEventPoster) in.GetLiveForConversationUseCase {
	return NewGetLiveForConversationHandler(convs, calls, calls, nodes, rooms, parts, events)
}

func TestGetLiveForConversation_IncludesParticipants(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	want := []out.ParticipantInfo{
		{UserID: memberID.String(), Role: "publisher", Muted: false, SelfMuted: true, TrackKinds: []string{"mic"}},
	}
	rooms := &fakeRoomControl{listParticipants: want}
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, &fakeParticipantLister{}, &fakeSystemEventPoster{})

	got, participants, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got != c {
		t.Fatalf("Handle returned a different call")
	}
	if len(participants) != 1 {
		t.Fatalf("participants = %+v, want 1 entry", participants)
	}
	gotP := participants[0]
	if gotP.UserID != want[0].UserID || gotP.Role != want[0].Role || gotP.Muted != want[0].Muted ||
		gotP.SelfMuted != want[0].SelfMuted || len(gotP.TrackKinds) != 1 || gotP.TrackKinds[0] != want[0].TrackKinds[0] {
		t.Fatalf("participants = %+v, want %+v", participants, want)
	}
}

func TestGetLiveForConversation_RemembersDepartedPublisher(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	joinedAt := time.Now().UTC()
	leftAt := joinedAt.Add(time.Minute)
	parts := &fakeParticipantLister{history: []*domain.Participant{{CallID: c.ID, UserID: memberID, Role: domain.RolePublisher, JoinedAt: joinedAt, LeftAt: &leftAt}}}
	rooms := &fakeRoomControl{listParticipants: []out.ParticipantInfo{}}
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, parts, &fakeSystemEventPoster{})

	_, participants, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != memberID.String() || participants[0].Connected || !participants[0].Accepted {
		t.Fatalf("participants = %+v, want departed accepted publisher", participants)
	}
}

func TestGetLiveForConversation_MediaNodeGone_EmptyNotError(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	nodes := &fakeMediaNodeRegistry{getErr: errors.New("node gone")}
	h := newHandler(calls, convs, nodes, &fakeRoomControl{}, &fakeParticipantLister{}, &fakeSystemEventPoster{})

	got, participants, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v, want nil (call row is still the source of truth)", err)
	}
	if got != c {
		t.Fatalf("Handle returned a different call")
	}
	if participants == nil || len(participants) != 0 {
		t.Fatalf("participants = %#v, want non-nil empty slice", participants)
	}
}

func TestGetLiveForConversation_ListParticipantsFails_EmptyNotError(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	rooms := &fakeRoomControl{listParticipantsErr: errors.New("sfu unreachable")}
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, &fakeParticipantLister{}, &fakeSystemEventPoster{})

	got, participants, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v, want nil", err)
	}
	if got != c {
		t.Fatalf("Handle returned a different call")
	}
	if participants == nil || len(participants) != 0 {
		t.Fatalf("participants = %#v, want non-nil empty slice", participants)
	}
}

func TestGetLiveForConversation_MissingRoomEndsCall(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	rooms := &fakeRoomControl{listParticipantsErr: out.ErrRoomNotFound}
	events := &fakeSystemEventPoster{}
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms, &fakeParticipantLister{}, events)

	_, _, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Handle error = %v, want not found", err)
	}
	if c.IsLive() {
		t.Fatal("missing media room left call live")
	}
	if len(events.events) != 1 || events.events[0] != domainmsg.SystemCallEnded {
		t.Fatalf("events = %v, want one call_ended", events.events)
	}
}

func TestGetLiveForConversation_NonMember(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	stranger := uuid.Must(uuid.NewV7())
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, &fakeParticipantLister{}, &fakeSystemEventPoster{})

	if _, _, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: stranger, ConversationID: convID}); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestGetLiveForConversation_NoLiveCall(t *testing.T) {
	calls := newFakeCallStore()
	convs := newFakeConversationReader()
	convID, memberID := groupChat(t, convs)
	h := newHandler(calls, convs, &fakeMediaNodeRegistry{}, &fakeRoomControl{}, &fakeParticipantLister{}, &fakeSystemEventPoster{})

	if _, _, err := h.Handle(context.Background(), in.GetLiveForConversationQuery{Actor: memberID, ConversationID: convID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
