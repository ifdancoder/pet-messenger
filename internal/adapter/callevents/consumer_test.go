package callevents

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
	usecasecall "pet-messenger/internal/usecase/call"
)

// ---- fakes ----

type fakeCallRepo struct {
	byID  map[uuid.UUID]*domaincall.Call
	parts map[uuid.UUID][]*domaincall.Participant
}

func newFakeCallRepo() *fakeCallRepo {
	return &fakeCallRepo{
		byID:  map[uuid.UUID]*domaincall.Call{},
		parts: map[uuid.UUID][]*domaincall.Participant{},
	}
}

func (f *fakeCallRepo) Create(_ context.Context, c *domaincall.Call) error {
	f.byID[c.ID] = c
	return nil
}
func (f *fakeCallRepo) GetByID(_ context.Context, id uuid.UUID) (*domaincall.Call, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domaincall.ErrNotFound
}
func (f *fakeCallRepo) GetLiveForConversation(_ context.Context, convID uuid.UUID) (*domaincall.Call, error) {
	for _, c := range f.byID {
		if c.ConversationID == convID && c.IsLive() {
			return c, nil
		}
	}
	return nil, domaincall.ErrNotFound
}
func (f *fakeCallRepo) MarkEnded(_ context.Context, id uuid.UUID, endedAt time.Time) (bool, error) {
	c, ok := f.byID[id]
	if !ok || !c.IsLive() {
		return false, nil
	}
	c.Status = domaincall.StatusEnded
	c.EndedAt = &endedAt
	return true, nil
}
func (f *fakeCallRepo) AddParticipant(_ context.Context, p *domaincall.Participant) error {
	f.parts[p.CallID] = append(f.parts[p.CallID], p)
	return nil
}
func (f *fakeCallRepo) MarkParticipantLeft(_ context.Context, callID, userID uuid.UUID, t time.Time) error {
	for _, p := range f.parts[callID] {
		if p.UserID == userID && p.LeftAt == nil {
			p.LeftAt = &t
		}
	}
	return nil
}
func (f *fakeCallRepo) ListParticipants(_ context.Context, callID uuid.UUID) ([]*domaincall.Participant, error) {
	return f.parts[callID], nil
}

// stubConvRepo satisfies conversation.Repository; the consumer only ever reaches
// GetByID (via finishCall), for which ErrNotFound is a tolerated no-op.
type stubConvRepo struct{}

func (stubConvRepo) Create(context.Context, *conversation.Conversation, []*conversation.Member) error {
	return nil
}
func (stubConvRepo) GetByID(context.Context, uuid.UUID) (*conversation.Conversation, error) {
	return nil, conversation.ErrNotFound
}
func (stubConvRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	return nil, conversation.ErrNotMember
}
func (stubConvRepo) GetDirect(context.Context, uuid.UUID, uuid.UUID) (*conversation.Conversation, error) {
	return nil, conversation.ErrNotFound
}
func (stubConvRepo) ListForUser(context.Context, uuid.UUID, time.Time, uuid.UUID, int) ([]*conversation.Conversation, error) {
	return nil, nil
}
func (stubConvRepo) AddMember(context.Context, *conversation.Member) error { return nil }
func (stubConvRepo) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stubConvRepo) SetMemberTier(context.Context, uuid.UUID, uuid.UUID, conversation.AdminTier) error {
	return nil
}
func (stubConvRepo) ListPublicChannels(context.Context, string, time.Time, uuid.UUID, int) ([]*conversation.Conversation, error) {
	return nil, nil
}
func (stubConvRepo) SetArchived(context.Context, uuid.UUID, bool) error { return nil }
func (stubConvRepo) RemoveMemberWithSuccession(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	return nil, nil
}
func (stubConvRepo) ClaimCreator(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	return nil, nil
}
func (stubConvRepo) ListMembers(context.Context, uuid.UUID, bool, time.Time, uuid.UUID, int) ([]*conversation.MemberProfile, error) {
	return nil, nil
}
func (stubConvRepo) ListMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

type stubRegistry struct{}

func (stubRegistry) PickLeastLoaded(context.Context) (usecasecall.Node, error) {
	return usecasecall.Node{}, usecasecall.ErrNoMediaNodes
}
func (stubRegistry) Get(context.Context, string) (usecasecall.Node, error) {
	return usecasecall.Node{ID: "node-a"}, nil
}

type stubRooms struct{}

func (stubRooms) CreateRoom(context.Context, usecasecall.Node, uuid.UUID) error { return nil }
func (stubRooms) CloseRoom(context.Context, usecasecall.Node, uuid.UUID) error  { return nil }
func (stubRooms) MuteParticipant(context.Context, usecasecall.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stubRooms) RemoveParticipant(context.Context, usecasecall.Node, uuid.UUID, uuid.UUID) error {
	return nil
}

type stubTickets struct{}

func (stubTickets) Issue(_, _ uuid.UUID, _ domaincall.Role, _ string) (string, time.Time, error) {
	return "tkt", time.Now().Add(time.Minute), nil
}

type stubICE struct{}

func (stubICE) ServersFor(uuid.UUID) []usecasecall.ICEServer { return nil }

type stubEvents struct{}

func (stubEvents) PostSystemEvent(context.Context, *conversation.Conversation, *uuid.UUID, message.SystemEvent, uuid.UUID, string) error {
	return nil
}

type stubNotifier struct{}

func (stubNotifier) CallStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }

func newConsumer(t *testing.T) (*Consumer, *fakeCallRepo) {
	t.Helper()
	calls := newFakeCallRepo()
	svc := usecasecall.NewService(calls, stubConvRepo{}, stubRegistry{}, stubRooms{}, stubTickets{}, stubICE{}, stubEvents{}, stubNotifier{})
	return NewConsumer(svc), calls
}

func liveCall(t *testing.T) *domaincall.Call {
	t.Helper()
	c, err := domaincall.New(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "node-a")
	if err != nil {
		t.Fatalf("domaincall.New: %v", err)
	}
	return c
}

// ---- tests ----

func TestConsumer_Ended_MarksCallEnded(t *testing.T) {
	cons, calls := newConsumer(t)
	c := liveCall(t)
	calls.byID[c.ID] = c

	cons.HandleMsg(&natsgo.Msg{Subject: "call." + c.ID.String() + ".ended", Data: []byte(`{}`)})

	if calls.byID[c.ID].IsLive() {
		t.Fatalf("call still live after call.<id>.ended")
	}
}

func TestConsumer_Ended_Idempotent(t *testing.T) {
	cons, calls := newConsumer(t)
	c := liveCall(t)
	calls.byID[c.ID] = c

	subj := "call." + c.ID.String() + ".ended"
	cons.HandleMsg(&natsgo.Msg{Subject: subj, Data: []byte(`{}`)})
	cons.HandleMsg(&natsgo.Msg{Subject: subj, Data: []byte(`{}`)})

	if calls.byID[c.ID].IsLive() {
		t.Fatalf("call still live")
	}
}

func TestConsumer_ParticipantLeft_MarksRowLeft(t *testing.T) {
	cons, calls := newConsumer(t)
	c := liveCall(t)
	calls.byID[c.ID] = c
	user := uuid.Must(uuid.NewV7())
	calls.parts[c.ID] = []*domaincall.Participant{{CallID: c.ID, UserID: user, JoinedAt: time.Now().UTC()}}

	cons.HandleMsg(&natsgo.Msg{
		Subject: "call." + c.ID.String() + ".participant_left",
		Data:    []byte(`{"user_id":"` + user.String() + `"}`),
	})

	if calls.parts[c.ID][0].LeftAt == nil {
		t.Fatalf("participant row not marked left")
	}
}

func TestConsumer_BadSubject_NoPanic(t *testing.T) {
	cons, _ := newConsumer(t)
	cons.HandleMsg(&natsgo.Msg{Subject: "not-a-call-subject"})
	cons.HandleMsg(&natsgo.Msg{Subject: "call.not-a-uuid.ended"})
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + uuid.Must(uuid.NewV7()).String() + ".weird"})
}

func TestConsumer_ParticipantLeft_BadPayload_NoPanic(t *testing.T) {
	cons, _ := newConsumer(t)
	id := uuid.Must(uuid.NewV7()).String()
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + id + ".participant_left", Data: []byte(`not json`)})
	cons.HandleMsg(&natsgo.Msg{Subject: "call." + id + ".participant_left", Data: []byte(`{"user_id":"nope"}`)})
}
