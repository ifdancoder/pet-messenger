package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

// fakeCallStore satisfies both out.CallWriter and out.CallLoader over one
// shared, in-memory call set.
type fakeCallStore struct {
	live       map[uuid.UUID]*domain.Call // conversationID -> live call
	byID       map[uuid.UUID]*domain.Call
	createErr  error
	created    []*domain.Call
	endedCalls []uuid.UUID
}

func newFakeCallStore() *fakeCallStore {
	return &fakeCallStore{live: map[uuid.UUID]*domain.Call{}, byID: map[uuid.UUID]*domain.Call{}}
}

func (f *fakeCallStore) Create(_ context.Context, c *domain.Call) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.live[c.ConversationID]; ok {
		return domain.ErrLiveCallExists
	}
	f.live[c.ConversationID] = c
	f.byID[c.ID] = c
	f.created = append(f.created, c)
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

func (f *fakeCallStore) ListLive(context.Context) ([]*domain.Call, error) {
	out := make([]*domain.Call, 0, len(f.live))
	for _, c := range f.live {
		out = append(out, c)
	}
	return out, nil
}

// seedLive registers a live call (byID + live index) without going
// through Create, so tests can start from an already-running call.
func (f *fakeCallStore) seedLive(c *domain.Call) {
	f.byID[c.ID] = c
	f.live[c.ConversationID] = c
}

// fakeParticipantStore satisfies both out.ParticipantWriter and
// out.ParticipantLister over one shared, in-memory participant set.
type fakeParticipantStore struct {
	parts      map[uuid.UUID][]*domain.Participant
	addPartErr error
}

func newFakeParticipantStore() *fakeParticipantStore {
	return &fakeParticipantStore{parts: map[uuid.UUID][]*domain.Participant{}}
}

func (f *fakeParticipantStore) AddParticipant(_ context.Context, p *domain.Participant) error {
	if f.addPartErr != nil {
		return f.addPartErr
	}
	for _, existing := range f.parts[p.CallID] {
		if existing.UserID == p.UserID && existing.LeftAt == nil {
			return domain.ErrAlreadyInCall
		}
	}
	f.parts[p.CallID] = append(f.parts[p.CallID], p)
	return nil
}

func (f *fakeParticipantStore) MarkParticipantLeft(_ context.Context, callID, userID uuid.UUID, t time.Time) error {
	for _, p := range f.parts[callID] {
		if p.UserID == userID && p.LeftAt == nil {
			p.LeftAt = &t
		}
	}
	return nil
}

func (f *fakeParticipantStore) ListParticipants(_ context.Context, callID uuid.UUID) ([]*domain.Participant, error) {
	return f.parts[callID], nil
}

// fakeConversationReader satisfies out.ConversationReader.
type fakeConversationReader struct {
	conv    *conversation.Conversation
	members map[uuid.UUID]*conversation.Member
	// extra lets a test give GetByID more than the one `conv` it can
	// resolve, for scenarios (like the reaper) that touch several
	// conversations at once.
	extra map[uuid.UUID]*conversation.Conversation
}

func newFakeConversationReader() *fakeConversationReader {
	return &fakeConversationReader{members: map[uuid.UUID]*conversation.Member{}}
}

func (f *fakeConversationReader) GetByID(_ context.Context, id uuid.UUID) (*conversation.Conversation, error) {
	if f.conv != nil && f.conv.ID == id {
		return f.conv, nil
	}
	if c, ok := f.extra[id]; ok {
		return c, nil
	}
	return nil, conversation.ErrNotFound
}

func (f *fakeConversationReader) GetMember(_ context.Context, _, userID uuid.UUID) (*conversation.Member, error) {
	if m, ok := f.members[userID]; ok {
		return m, nil
	}
	return nil, conversation.ErrNotMember
}

// fakeMediaNodeRegistry satisfies out.MediaNodeRegistry.
type fakeMediaNodeRegistry struct {
	node    out.Node
	pickErr error
	getErr  error
}

func (f *fakeMediaNodeRegistry) PickLeastLoaded(context.Context) (out.Node, error) {
	if f.pickErr != nil {
		return out.Node{}, f.pickErr
	}
	return f.node, nil
}

func (f *fakeMediaNodeRegistry) Get(context.Context, string) (out.Node, error) {
	if f.getErr != nil {
		return out.Node{}, f.getErr
	}
	return f.node, nil
}

// fakeRoomControl satisfies out.RoomControl.
type fakeRoomControl struct {
	createErr                                error
	created, closed, muted, unmuted, removed []string
	listParticipants                         []out.ParticipantInfo
	listParticipantsErr                      error
}

func (f *fakeRoomControl) CreateRoom(_ context.Context, _ out.Node, id uuid.UUID) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, id.String())
	return nil
}

func (f *fakeRoomControl) CloseRoom(_ context.Context, _ out.Node, id uuid.UUID) error {
	f.closed = append(f.closed, id.String())
	return nil
}

func (f *fakeRoomControl) MuteParticipant(_ context.Context, _ out.Node, id, u uuid.UUID) error {
	f.muted = append(f.muted, id.String()+"/"+u.String())
	return nil
}

func (f *fakeRoomControl) UnmuteParticipant(_ context.Context, _ out.Node, id, u uuid.UUID) error {
	f.unmuted = append(f.unmuted, id.String()+"/"+u.String())
	return nil
}

func (f *fakeRoomControl) RemoveParticipant(_ context.Context, _ out.Node, id, u uuid.UUID) error {
	f.removed = append(f.removed, id.String()+"/"+u.String())
	return nil
}

func (f *fakeRoomControl) ListParticipants(_ context.Context, _ out.Node, _ uuid.UUID) ([]out.ParticipantInfo, error) {
	if f.listParticipantsErr != nil {
		return nil, f.listParticipantsErr
	}
	return f.listParticipants, nil
}

// fakeJoinTicketIssuer satisfies out.JoinTicketIssuer.
type fakeJoinTicketIssuer struct{}

func (fakeJoinTicketIssuer) Issue(_, _ uuid.UUID, r domain.Role, _ string) (string, time.Time, error) {
	return "tkt-" + string(r), time.Now().Add(time.Minute), nil
}

// fakeICEConfigProvider satisfies out.ICEConfigProvider.
type fakeICEConfigProvider struct{}

func (fakeICEConfigProvider) ServersFor(uuid.UUID) []out.ICEServer {
	return []out.ICEServer{{URLs: []string{"stun:x"}}}
}

// fakeSystemEventPoster satisfies out.SystemEventPoster.
type fakeSystemEventPoster struct{ events []domainmsg.SystemEvent }

func (f *fakeSystemEventPoster) PostSystemEvent(_ context.Context, _ *conversation.Conversation, _ *uuid.UUID, e domainmsg.SystemEvent, _ uuid.UUID, _ string) error {
	f.events = append(f.events, e)
	return nil
}

// fakeCallNotifier satisfies out.CallNotifier.
type fakeCallNotifier struct{ started int }

func (f *fakeCallNotifier) CallStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.started++
	return nil
}

// groupChat seeds convs with a fresh group-chat conversation and one
// member, returning their ids.
func groupChat(t *testing.T, convs *fakeConversationReader) (convID, memberID uuid.UUID) {
	t.Helper()
	convID = uuid.Must(uuid.NewV7())
	memberID = uuid.Must(uuid.NewV7())
	convs.conv = &conversation.Conversation{ID: convID, Kind: conversation.KindGroupChat}
	convs.members[memberID] = &conversation.Member{ConversationID: convID, UserID: memberID, Role: conversation.RoleMember}
	return
}

// addMember adds a fresh member at the given admin tier to convID and
// returns their id.
func addMember(convs *fakeConversationReader, convID uuid.UUID, tier conversation.AdminTier) uuid.UUID {
	id := uuid.Must(uuid.NewV7())
	convs.members[id] = &conversation.Member{
		ConversationID: convID,
		UserID:         id,
		Role:           conversation.RoleMember,
		AdminTier:      tier,
	}
	return id
}

// mustCall builds a fresh domain.Call or fails the test.
func mustCall(t *testing.T, convID, initiatorID uuid.UUID) *domain.Call {
	t.Helper()
	c, err := domain.New(convID, initiatorID, "node-a")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	return c
}
