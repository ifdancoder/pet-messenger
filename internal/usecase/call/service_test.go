package call

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

// ---- fakes ----

type fakeCallRepo struct {
	live       map[uuid.UUID]*domaincall.Call // conversationID -> live call
	byID       map[uuid.UUID]*domaincall.Call
	createErr  error
	created    []*domaincall.Call
	parts      map[uuid.UUID][]*domaincall.Participant
	addPartErr error
	endedCalls []uuid.UUID
}

func newFakeCallRepo() *fakeCallRepo {
	return &fakeCallRepo{
		live:  map[uuid.UUID]*domaincall.Call{},
		byID:  map[uuid.UUID]*domaincall.Call{},
		parts: map[uuid.UUID][]*domaincall.Participant{},
	}
}

func (f *fakeCallRepo) ListLive(context.Context) ([]*domaincall.Call, error) {
	out := make([]*domaincall.Call, 0, len(f.live))
	for _, c := range f.live {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCallRepo) Create(_ context.Context, c *domaincall.Call) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.live[c.ConversationID]; ok {
		return domaincall.ErrLiveCallExists
	}
	f.live[c.ConversationID] = c
	f.byID[c.ID] = c
	f.created = append(f.created, c)
	return nil
}

func (f *fakeCallRepo) GetByID(_ context.Context, id uuid.UUID) (*domaincall.Call, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domaincall.ErrNotFound
}

func (f *fakeCallRepo) GetLiveForConversation(_ context.Context, convID uuid.UUID) (*domaincall.Call, error) {
	if c, ok := f.live[convID]; ok && c.IsLive() {
		return c, nil
	}
	return nil, domaincall.ErrNotFound
}

func (f *fakeCallRepo) MarkEnded(_ context.Context, id uuid.UUID, _ time.Time) (bool, error) {
	c, ok := f.byID[id]
	if !ok || !c.IsLive() {
		return false, nil
	}
	_ = c.End()
	delete(f.live, c.ConversationID)
	f.endedCalls = append(f.endedCalls, id)
	return true, nil
}

func (f *fakeCallRepo) AddParticipant(_ context.Context, p *domaincall.Participant) error {
	if f.addPartErr != nil {
		return f.addPartErr
	}
	for _, existing := range f.parts[p.CallID] {
		if existing.UserID == p.UserID && existing.LeftAt == nil {
			return domaincall.ErrAlreadyInCall
		}
	}
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

// seedLive registers a live call (byID + live index) without going through
// Create, so tests can start from an already-running call.
func (f *fakeCallRepo) seedLive(c *domaincall.Call) {
	f.byID[c.ID] = c
	f.live[c.ConversationID] = c
}

type fakeConvRepo struct {
	conv    *conversation.Conversation
	members map[uuid.UUID]*conversation.Member
	// extra lets a test give GetByID more than the one `conv` it can resolve,
	// for scenarios (like the reaper) that touch several conversations at once.
	extra map[uuid.UUID]*conversation.Conversation
}

func (f *fakeConvRepo) Create(context.Context, *conversation.Conversation, []*conversation.Member) error {
	return nil
}
func (f *fakeConvRepo) GetByID(_ context.Context, id uuid.UUID) (*conversation.Conversation, error) {
	if f.conv != nil && f.conv.ID == id {
		return f.conv, nil
	}
	if c, ok := f.extra[id]; ok {
		return c, nil
	}
	return nil, conversation.ErrNotFound
}
func (f *fakeConvRepo) GetMember(_ context.Context, _, userID uuid.UUID) (*conversation.Member, error) {
	if m, ok := f.members[userID]; ok {
		return m, nil
	}
	return nil, conversation.ErrNotMember
}
func (f *fakeConvRepo) GetDirect(context.Context, uuid.UUID, uuid.UUID) (*conversation.Conversation, error) {
	return nil, conversation.ErrNotFound
}
func (f *fakeConvRepo) ListForUser(context.Context, uuid.UUID, time.Time, uuid.UUID, int) ([]*conversation.Conversation, error) {
	return nil, nil
}
func (f *fakeConvRepo) AddMember(context.Context, *conversation.Member) error { return nil }
func (f *fakeConvRepo) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeConvRepo) SetMemberTier(context.Context, uuid.UUID, uuid.UUID, conversation.AdminTier) error {
	return nil
}
func (f *fakeConvRepo) SetLastReadMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeConvRepo) ListPublicChannels(context.Context, string, time.Time, uuid.UUID, int) ([]*conversation.Conversation, error) {
	return nil, nil
}
func (f *fakeConvRepo) SetArchived(context.Context, uuid.UUID, bool) error { return nil }
func (f *fakeConvRepo) RemoveMemberWithSuccession(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	return nil, nil
}
func (f *fakeConvRepo) ClaimCreator(context.Context, uuid.UUID, uuid.UUID) (*conversation.Member, error) {
	return nil, nil
}
func (f *fakeConvRepo) ListMembers(context.Context, uuid.UUID, bool, time.Time, uuid.UUID, int) ([]*conversation.MemberProfile, error) {
	return nil, nil
}
func (f *fakeConvRepo) ListMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

type fakeRegistry struct {
	node    Node
	pickErr error
	getErr  error
}

func (f *fakeRegistry) PickLeastLoaded(context.Context) (Node, error) {
	if f.pickErr != nil {
		return Node{}, f.pickErr
	}
	return f.node, nil
}
func (f *fakeRegistry) Get(context.Context, string) (Node, error) {
	if f.getErr != nil {
		return Node{}, f.getErr
	}
	return f.node, nil
}

type fakeRooms struct {
	createErr                                error
	created, closed, muted, unmuted, removed []string
	listParticipants                         []ParticipantInfo
	listParticipantsErr                      error
}

func (f *fakeRooms) CreateRoom(_ context.Context, _ Node, id uuid.UUID) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, id.String())
	return nil
}
func (f *fakeRooms) CloseRoom(_ context.Context, _ Node, id uuid.UUID) error {
	f.closed = append(f.closed, id.String())
	return nil
}
func (f *fakeRooms) MuteParticipant(_ context.Context, _ Node, id, u uuid.UUID) error {
	f.muted = append(f.muted, id.String()+"/"+u.String())
	return nil
}

func (f *fakeRooms) UnmuteParticipant(_ context.Context, _ Node, id, u uuid.UUID) error {
	f.unmuted = append(f.unmuted, id.String()+"/"+u.String())
	return nil
}
func (f *fakeRooms) RemoveParticipant(_ context.Context, _ Node, id, u uuid.UUID) error {
	f.removed = append(f.removed, id.String()+"/"+u.String())
	return nil
}
func (f *fakeRooms) ListParticipants(_ context.Context, _ Node, _ uuid.UUID) ([]ParticipantInfo, error) {
	if f.listParticipantsErr != nil {
		return nil, f.listParticipantsErr
	}
	return f.listParticipants, nil
}

type fakeTickets struct{}

func (fakeTickets) Issue(_, _ uuid.UUID, r domaincall.Role, _ string) (string, time.Time, error) {
	return "tkt-" + string(r), time.Now().Add(time.Minute), nil
}

type fakeICE struct{}

func (fakeICE) ServersFor(uuid.UUID) []ICEServer { return []ICEServer{{URLs: []string{"stun:x"}}} }

type fakeEvents struct{ events []message.SystemEvent }

func (f *fakeEvents) PostSystemEvent(_ context.Context, _ *conversation.Conversation, _ *uuid.UUID, e message.SystemEvent, _ uuid.UUID, _ string) error {
	f.events = append(f.events, e)
	return nil
}

type fakeNotifier struct{ started int }

func (f *fakeNotifier) CallStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.started++
	return nil
}

// ---- harness ----

func newSvc(t *testing.T) (*Service, *fakeCallRepo, *fakeConvRepo, *fakeRegistry, *fakeRooms, *fakeEvents, *fakeNotifier) {
	t.Helper()
	calls := newFakeCallRepo()
	convs := &fakeConvRepo{members: map[uuid.UUID]*conversation.Member{}}
	reg := &fakeRegistry{node: Node{ID: "node-a", GRPCAddr: "grpc:1", WSAddr: "ws:2"}}
	rooms := &fakeRooms{}
	events := &fakeEvents{}
	notif := &fakeNotifier{}
	s := NewService(calls, convs, reg, rooms, fakeTickets{}, fakeICE{}, events, notif)
	return s, calls, convs, reg, rooms, events, notif
}

func groupChat(t *testing.T, convs *fakeConvRepo) (convID, memberID uuid.UUID) {
	t.Helper()
	convID = uuid.Must(uuid.NewV7())
	memberID = uuid.Must(uuid.NewV7())
	convs.conv = &conversation.Conversation{ID: convID, Kind: conversation.KindGroupChat}
	convs.members[memberID] = &conversation.Member{ConversationID: convID, UserID: memberID, Role: conversation.RoleMember}
	return
}

func addMember(convs *fakeConvRepo, convID uuid.UUID, tier conversation.AdminTier) uuid.UUID {
	id := uuid.Must(uuid.NewV7())
	convs.members[id] = &conversation.Member{
		ConversationID: convID,
		UserID:         id,
		Role:           conversation.RoleMember,
		AdminTier:      tier,
	}
	return id
}

func mustCall(t *testing.T, convID, initiatorID uuid.UUID) *domaincall.Call {
	t.Helper()
	c, err := domaincall.New(convID, initiatorID, "node-a")
	if err != nil {
		t.Fatalf("domaincall.New: %v", err)
	}
	return c
}

// ---- StartCall ----

func TestStartCall_HappyPath(t *testing.T) {
	s, calls, convs, _, rooms, events, notif := newSvc(t)
	convID, memberID := groupChat(t, convs)

	c, ji, err := s.StartCall(context.Background(), memberID, convID)
	if err != nil {
		t.Fatalf("StartCall: %v", err)
	}
	if len(calls.created) != 1 {
		t.Fatalf("calls.created = %d, want 1", len(calls.created))
	}
	if len(rooms.created) != 1 {
		t.Fatalf("rooms.created = %d, want 1", len(rooms.created))
	}
	if got := calls.parts[c.ID]; len(got) != 1 || got[0].UserID != memberID {
		t.Fatalf("participant rows = %+v, want one for the actor", got)
	}
	if len(events.events) != 1 || events.events[0] != message.SystemCallStarted {
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
	if ji.Role != domaincall.RolePublisher {
		t.Fatalf("ji.Role = %q, want publisher", ji.Role)
	}
}

func TestStartCall_LiveCallExists(t *testing.T) {
	s, calls, convs, _, rooms, events, notif := newSvc(t)
	convID, memberID := groupChat(t, convs)
	existing := mustCall(t, convID, addMember(convs, convID, conversation.TierNone))
	calls.seedLive(existing)

	got, ji, err := s.StartCall(context.Background(), memberID, convID)
	if err != nil {
		t.Fatalf("StartCall: %v", err)
	}
	if got != existing {
		t.Fatalf("StartCall returned a different call")
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
	s, _, convs, _, _, _, _ := newSvc(t)
	convID := uuid.Must(uuid.NewV7())
	memberID := uuid.Must(uuid.NewV7())
	convs.conv = &conversation.Conversation{ID: convID, Kind: conversation.KindChannelBroadcast}
	convs.members[memberID] = &conversation.Member{ConversationID: convID, UserID: memberID}

	if _, _, err := s.StartCall(context.Background(), memberID, convID); !errors.Is(err, ErrCallNotSupported) {
		t.Fatalf("err = %v, want ErrCallNotSupported", err)
	}
}

func TestStartCall_ArchivedConversation(t *testing.T) {
	s, _, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	now := time.Now().UTC()
	convs.conv.ArchivedAt = &now

	if _, _, err := s.StartCall(context.Background(), memberID, convID); !errors.Is(err, conversation.ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestStartCall_NonMember(t *testing.T) {
	s, _, convs, _, _, _, _ := newSvc(t)
	convID, _ := groupChat(t, convs)
	stranger := uuid.Must(uuid.NewV7())

	if _, _, err := s.StartCall(context.Background(), stranger, convID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestStartCall_NoMediaNodes(t *testing.T) {
	s, _, convs, reg, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	reg.pickErr = ErrNoMediaNodes

	if _, _, err := s.StartCall(context.Background(), memberID, convID); !errors.Is(err, ErrNoMediaNodes) {
		t.Fatalf("err = %v, want ErrNoMediaNodes", err)
	}
}

func TestStartCall_RoomCreateFailsCompensates(t *testing.T) {
	s, calls, convs, _, rooms, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	rooms.createErr = errors.New("sfu down")

	if _, _, err := s.StartCall(context.Background(), memberID, convID); !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("err = %v, want ErrMediaUnavailable", err)
	}
	if len(calls.endedCalls) != 1 {
		t.Fatalf("calls.endedCalls = %v, want the compensated call", calls.endedCalls)
	}
	if len(calls.live) != 0 {
		t.Fatalf("a live call still remains: %v", calls.live)
	}
}

// ---- JoinCall ----

func TestJoinCall_HappyPath(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)
	joinerID := addMember(convs, convID, conversation.TierNone)

	ji, err := s.JoinCall(context.Background(), joinerID, c.ID, domaincall.RoleViewer)
	if err != nil {
		t.Fatalf("JoinCall: %v", err)
	}
	if ji.Role != domaincall.RoleViewer {
		t.Fatalf("ji.Role = %q, want viewer", ji.Role)
	}
	var found bool
	for _, p := range calls.parts[c.ID] {
		if p.UserID == joinerID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no participant row added for the joiner")
	}
}

func TestJoinCall_NotLive(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	_ = c.End()
	calls.byID[c.ID] = c

	if _, err := s.JoinCall(context.Background(), initiatorID, c.ID, domaincall.RolePublisher); !errors.Is(err, domaincall.ErrCallNotLive) {
		t.Fatalf("err = %v, want ErrCallNotLive", err)
	}
}

func TestJoinCall_AlreadyParticipantReconnects(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	calls.addPartErr = domaincall.ErrAlreadyInCall

	ji, err := s.JoinCall(context.Background(), memberID, c.ID, domaincall.RolePublisher)
	if err != nil {
		t.Fatalf("JoinCall: %v, want nil (reconnect)", err)
	}
	if ji.Ticket == "" {
		t.Fatalf("expected a JoinInfo with a ticket on reconnect")
	}
}

func TestJoinCall_MediaNodeGone(t *testing.T) {
	s, calls, convs, reg, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	reg.getErr = ErrNoMediaNodes

	if _, err := s.JoinCall(context.Background(), memberID, c.ID, domaincall.RolePublisher); !errors.Is(err, ErrMediaNodeGone) {
		t.Fatalf("err = %v, want ErrMediaNodeGone", err)
	}
}

// ---- GetLiveForConversation ----

func TestGetLiveForConversation_IncludesParticipants(t *testing.T) {
	s, calls, convs, _, rooms, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	want := []ParticipantInfo{
		{UserID: memberID.String(), Role: "publisher", Muted: false, SelfMuted: true, TrackKinds: []string{"mic"}},
	}
	rooms.listParticipants = want

	got, participants, err := s.GetLiveForConversation(context.Background(), memberID, convID)
	if err != nil {
		t.Fatalf("GetLiveForConversation: %v", err)
	}
	if got != c {
		t.Fatalf("GetLiveForConversation returned a different call")
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

func TestGetLiveForConversation_MediaNodeGone_EmptyNotError(t *testing.T) {
	s, calls, convs, reg, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	reg.getErr = errors.New("node gone")

	got, participants, err := s.GetLiveForConversation(context.Background(), memberID, convID)
	if err != nil {
		t.Fatalf("GetLiveForConversation: %v, want nil (call row is still the source of truth)", err)
	}
	if got != c {
		t.Fatalf("GetLiveForConversation returned a different call")
	}
	if participants == nil || len(participants) != 0 {
		t.Fatalf("participants = %#v, want non-nil empty slice", participants)
	}
}

func TestGetLiveForConversation_ListParticipantsFails_EmptyNotError(t *testing.T) {
	s, calls, convs, _, rooms, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	rooms.listParticipantsErr = errors.New("sfu unreachable")

	got, participants, err := s.GetLiveForConversation(context.Background(), memberID, convID)
	if err != nil {
		t.Fatalf("GetLiveForConversation: %v, want nil", err)
	}
	if got != c {
		t.Fatalf("GetLiveForConversation returned a different call")
	}
	if participants == nil || len(participants) != 0 {
		t.Fatalf("participants = %#v, want non-nil empty slice", participants)
	}
}

func TestGetLiveForConversation_MissingRoomEndsCall(t *testing.T) {
	s, calls, convs, _, rooms, events, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	rooms.listParticipantsErr = ErrRoomNotFound

	_, _, err := s.GetLiveForConversation(context.Background(), memberID, convID)
	if !errors.Is(err, domaincall.ErrNotFound) {
		t.Fatalf("GetLiveForConversation error = %v, want not found", err)
	}
	if c.IsLive() {
		t.Fatal("missing media room left call live")
	}
	if len(events.events) != 1 || events.events[0] != message.SystemCallEnded {
		t.Fatalf("events = %v, want one call_ended", events.events)
	}
}

func TestGetLiveForConversation_NonMember(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	stranger := uuid.Must(uuid.NewV7())

	if _, _, err := s.GetLiveForConversation(context.Background(), stranger, convID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestGetLiveForConversation_NoLiveCall(t *testing.T) {
	s, _, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)

	if _, _, err := s.GetLiveForConversation(context.Background(), memberID, convID); !errors.Is(err, domaincall.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ---- EndCall ----

func TestEndCall_ByInitiator(t *testing.T) {
	s, calls, convs, _, rooms, events, _ := newSvc(t)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)

	if err := s.EndCall(context.Background(), initiatorID, c.ID); err != nil {
		t.Fatalf("EndCall: %v", err)
	}
	if len(rooms.closed) != 1 {
		t.Fatalf("rooms.closed = %d, want 1", len(rooms.closed))
	}
	if len(calls.endedCalls) != 1 {
		t.Fatalf("calls.endedCalls = %v, want the ended call", calls.endedCalls)
	}
	if c.IsLive() {
		t.Fatalf("call still live after EndCall")
	}
	var sawEnded bool
	for _, e := range events.events {
		if e == message.SystemCallEnded {
			sawEnded = true
		}
	}
	if !sawEnded {
		t.Fatalf("events = %v, want a call_ended", events.events)
	}
}

func TestEndCall_ByAdminTierMember(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, _ := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)
	adminID := addMember(convs, convID, conversation.TierAdmin)

	if err := s.EndCall(context.Background(), adminID, c.ID); err != nil {
		t.Fatalf("EndCall by admin: %v", err)
	}
	if c.IsLive() {
		t.Fatalf("call still live after EndCall")
	}
}

func TestEndCall_ByPlainMemberForbidden(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
	calls.seedLive(c)

	if err := s.EndCall(context.Background(), memberID, c.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestEndCall_AlreadyEnded(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	_ = c.End()
	calls.byID[c.ID] = c

	if err := s.EndCall(context.Background(), initiatorID, c.ID); !errors.Is(err, domaincall.ErrAlreadyEnded) {
		t.Fatalf("err = %v, want ErrAlreadyEnded", err)
	}
}

// ---- MuteParticipant / RemoveParticipant ----

func TestModerateParticipant(t *testing.T) {
	t.Run("initiator may mute", func(t *testing.T) {
		s, calls, convs, _, rooms, _, _ := newSvc(t)
		convID, _ := groupChat(t, convs)
		c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
		calls.seedLive(c)
		target := addMember(convs, convID, conversation.TierNone)

		if err := s.MuteParticipant(context.Background(), c.InitiatorID, c.ID, target); err != nil {
			t.Fatalf("MuteParticipant: %v", err)
		}
		if len(rooms.muted) != 1 {
			t.Fatalf("rooms.muted = %d, want 1", len(rooms.muted))
		}
	})

	t.Run("admin may remove and the row is marked left", func(t *testing.T) {
		s, calls, convs, _, rooms, _, _ := newSvc(t)
		convID, _ := groupChat(t, convs)
		c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
		calls.seedLive(c)
		adminID := addMember(convs, convID, conversation.TierAdmin)
		target := addMember(convs, convID, conversation.TierNone)
		calls.parts[c.ID] = []*domaincall.Participant{{CallID: c.ID, UserID: target}}

		if err := s.RemoveParticipant(context.Background(), adminID, c.ID, target); err != nil {
			t.Fatalf("RemoveParticipant: %v", err)
		}
		if len(rooms.removed) != 1 {
			t.Fatalf("rooms.removed = %d, want 1", len(rooms.removed))
		}
		if calls.parts[c.ID][0].LeftAt == nil {
			t.Fatalf("target participant row not marked left")
		}
	})

	t.Run("plain member is forbidden", func(t *testing.T) {
		s, calls, convs, _, _, _, _ := newSvc(t)
		convID, memberID := groupChat(t, convs)
		c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
		calls.seedLive(c)
		target := addMember(convs, convID, conversation.TierNone)

		if err := s.MuteParticipant(context.Background(), memberID, c.ID, target); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("cannot target self", func(t *testing.T) {
		s, calls, convs, _, _, _, _ := newSvc(t)
		convID, _ := groupChat(t, convs)
		c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
		calls.seedLive(c)

		if err := s.MuteParticipant(context.Background(), c.InitiatorID, c.ID, c.InitiatorID); !errors.Is(err, ErrCannotTargetSelf) {
			t.Fatalf("err = %v, want ErrCannotTargetSelf", err)
		}
	})
}

// ---- NATS event handlers ----

func TestHandleCallEnded_Idempotent(t *testing.T) {
	s, calls, convs, _, _, events, _ := newSvc(t)
	convID, initiatorID := groupChat(t, convs)
	c := mustCall(t, convID, initiatorID)
	calls.seedLive(c)

	if err := s.HandleCallEnded(context.Background(), c.ID); err != nil {
		t.Fatalf("HandleCallEnded #1: %v", err)
	}
	if err := s.HandleCallEnded(context.Background(), c.ID); err != nil {
		t.Fatalf("HandleCallEnded #2: %v", err)
	}
	var ended int
	for _, e := range events.events {
		if e == message.SystemCallEnded {
			ended++
		}
	}
	if ended != 1 {
		t.Fatalf("call_ended posted %d times, want exactly 1", ended)
	}
}

func TestHandleParticipantLeft(t *testing.T) {
	s, calls, convs, _, _, _, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	now := time.Now().UTC()
	calls.parts[c.ID] = []*domaincall.Participant{{CallID: c.ID, UserID: memberID, JoinedAt: now}}

	if err := s.HandleParticipantLeft(context.Background(), c.ID, memberID); err != nil {
		t.Fatalf("HandleParticipantLeft: %v", err)
	}
	if calls.parts[c.ID][0].LeftAt == nil {
		t.Fatalf("participant row not marked left")
	}
	// Unknown call / user is not an error.
	if err := s.HandleParticipantLeft(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("HandleParticipantLeft (unknown): %v", err)
	}
}

func TestReapStaleCalls(t *testing.T) {
	s, calls, convs, reg, _, events, _ := newSvc(t)
	conv1ID, user1ID := groupChat(t, convs)
	conv2ID := uuid.Must(uuid.NewV7())
	convs.extra = map[uuid.UUID]*conversation.Conversation{
		conv2ID: {ID: conv2ID, Kind: conversation.KindGroupChat},
	}

	c1 := mustCall(t, conv1ID, user1ID)
	c2 := mustCall(t, conv2ID, user1ID)
	calls.seedLive(c1)
	calls.seedLive(c2)

	reg.getErr = errors.New("node gone")

	n, err := s.ReapStaleCalls(context.Background())
	if err != nil {
		t.Fatalf("ReapStaleCalls: %v", err)
	}
	if n != 2 {
		t.Fatalf("ReapStaleCalls = %d, want 2", n)
	}
	if calls.byID[c1.ID].IsLive() || calls.byID[c2.ID].IsLive() {
		t.Fatalf("both calls should be ended: %+v %+v", calls.byID[c1.ID], calls.byID[c2.ID])
	}
	var ended int
	for _, e := range events.events {
		if e == message.SystemCallEnded {
			ended++
		}
	}
	if ended != 2 {
		t.Fatalf("call_ended posted %d times, want 2", ended)
	}

	// Idempotent: nothing is live anymore, so a second sweep ends nothing.
	n, err = s.ReapStaleCalls(context.Background())
	if err != nil {
		t.Fatalf("ReapStaleCalls (2nd): %v", err)
	}
	if n != 0 {
		t.Fatalf("ReapStaleCalls (2nd) = %d, want 0", n)
	}
}

func TestReapStaleCalls_MissingRoom(t *testing.T) {
	s, calls, convs, _, rooms, events, _ := newSvc(t)
	convID, memberID := groupChat(t, convs)
	c := mustCall(t, convID, memberID)
	calls.seedLive(c)
	rooms.listParticipantsErr = ErrRoomNotFound

	n, err := s.ReapStaleCalls(context.Background())
	if err != nil {
		t.Fatalf("ReapStaleCalls: %v", err)
	}
	if n != 1 || c.IsLive() {
		t.Fatalf("ReapStaleCalls = %d, call live = %v; want 1, false", n, c.IsLive())
	}
	if len(events.events) != 1 || events.events[0] != message.SystemCallEnded {
		t.Fatalf("events = %v, want one call_ended", events.events)
	}
}

func TestUnmuteParticipant_Authorization(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tier            conversation.AdminTier
		initiator, self bool
		want            error
	}{
		{name: "initiator", initiator: true},
		{name: "admin", tier: conversation.TierAdmin},
		{name: "plain member", tier: conversation.TierNone, want: ErrForbidden},
		{name: "self unmute denied", initiator: true, self: true, want: ErrCannotTargetSelf},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, calls, convs, _, rooms, _, _ := newSvc(t)
			convID, _ := groupChat(t, convs)
			c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
			calls.seedLive(c)
			actor := addMember(convs, convID, tc.tier)
			if tc.initiator {
				actor = c.InitiatorID
			}
			target := addMember(convs, convID, conversation.TierNone)
			if tc.self {
				target = actor
			}
			err := s.UnmuteParticipant(context.Background(), actor, c.ID, target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("UnmuteParticipant = %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				if len(rooms.unmuted) != 0 {
					t.Fatal("unauthorized request reached media node")
				}
				return
			}
			if len(rooms.unmuted) != 1 || rooms.unmuted[0] != c.ID.String()+"/"+target.String() {
				t.Fatalf("unmuted = %v", rooms.unmuted)
			}
		})
	}
}
