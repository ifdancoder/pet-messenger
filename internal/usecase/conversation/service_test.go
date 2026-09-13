package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
	"pet-messenger/internal/domain/user"
)

type fakeConvRepo struct {
	created        []*domainconv.Conversation
	getDirect      func(a, b uuid.UUID) (*domainconv.Conversation, error)
	createErr      error
	listCalledWith struct {
		before   time.Time
		beforeID uuid.UUID
		limit    int
	}
	listResult []*domainconv.Conversation

	conv          *domainconv.Conversation
	convErr       error
	member        *domainconv.Member
	memberErr     error
	getMemberFunc func(ctx context.Context, conversationID, userID uuid.UUID) (*domainconv.Member, error)

	added      []*domainconv.Member
	addErr     error
	removed    [][2]uuid.UUID
	removeErr  error
	publicList []*domainconv.Conversation
	gotQuery   string
	gotLimit   int

	tierSet []tierSetCall
	tierErr error

	archivedSet   []archivedCall
	archivedErr   error
	succession    func(cid, uid uuid.UUID) (*domainconv.Member, error) // RemoveMemberWithSuccession
	claim         func(cid, uid uuid.UUID) (*domainconv.Member, error) // ClaimCreator
	memberList    []*domainconv.MemberProfile
	gotAdminsOnly bool
	memberIDs     []uuid.UUID
}

type tierSetCall struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	Tier           domainconv.AdminTier
}

type archivedCall struct {
	ConversationID uuid.UUID
	Archived       bool
}

func (f *fakeConvRepo) Create(_ context.Context, c *domainconv.Conversation, _ []*domainconv.Member) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, c)
	return nil
}
func (f *fakeConvRepo) GetByID(context.Context, uuid.UUID) (*domainconv.Conversation, error) {
	if f.convErr != nil {
		return nil, f.convErr
	}
	if f.conv != nil {
		return f.conv, nil
	}
	return nil, domainconv.ErrNotFound
}
func (f *fakeConvRepo) GetMember(ctx context.Context, conversationID, userID uuid.UUID) (*domainconv.Member, error) {
	if f.getMemberFunc != nil {
		return f.getMemberFunc(ctx, conversationID, userID)
	}
	if f.memberErr != nil {
		return nil, f.memberErr
	}
	if f.member != nil {
		return f.member, nil
	}
	return nil, domainconv.ErrNotMember
}
func (f *fakeConvRepo) GetDirect(_ context.Context, a, b uuid.UUID) (*domainconv.Conversation, error) {
	if f.getDirect != nil {
		return f.getDirect(a, b)
	}
	return nil, domainconv.ErrNotFound
}
func (f *fakeConvRepo) ListForUser(_ context.Context, _ uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	f.listCalledWith.before = before
	f.listCalledWith.beforeID = beforeID
	f.listCalledWith.limit = limit
	return f.listResult, nil
}
func (f *fakeConvRepo) AddMember(_ context.Context, m *domainconv.Member) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, m)
	return nil
}
func (f *fakeConvRepo) RemoveMember(_ context.Context, conversationID, userID uuid.UUID) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, [2]uuid.UUID{conversationID, userID})
	return nil
}
func (f *fakeConvRepo) ListPublicChannels(_ context.Context, query string, _ time.Time, _ uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	f.gotQuery, f.gotLimit = query, limit
	return f.publicList, nil
}
func (f *fakeConvRepo) SetMemberTier(_ context.Context, conversationID, userID uuid.UUID, tier domainconv.AdminTier) error {
	if f.tierErr != nil {
		return f.tierErr
	}
	f.tierSet = append(f.tierSet, tierSetCall{conversationID, userID, tier})
	return nil
}
func (f *fakeConvRepo) SetLastReadMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeConvRepo) SetArchived(_ context.Context, conversationID uuid.UUID, archived bool) error {
	if f.archivedErr != nil {
		return f.archivedErr
	}
	f.archivedSet = append(f.archivedSet, archivedCall{conversationID, archived})
	return nil
}
func (f *fakeConvRepo) RemoveMemberWithSuccession(_ context.Context, cid, uid uuid.UUID) (*domainconv.Member, error) {
	if f.succession != nil {
		return f.succession(cid, uid)
	}
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	f.removed = append(f.removed, [2]uuid.UUID{cid, uid})
	return nil, nil
}
func (f *fakeConvRepo) ClaimCreator(_ context.Context, cid, uid uuid.UUID) (*domainconv.Member, error) {
	if f.claim != nil {
		return f.claim(cid, uid)
	}
	return nil, nil
}
func (f *fakeConvRepo) ListMembers(_ context.Context, _ uuid.UUID, adminsOnly bool, _ time.Time, _ uuid.UUID, _ int) ([]*domainconv.MemberProfile, error) {
	f.gotAdminsOnly = adminsOnly
	return f.memberList, nil
}
func (f *fakeConvRepo) ListMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.memberIDs, nil
}

type fakeUserRepo struct {
	exists map[uuid.UUID]bool
}

func (f *fakeUserRepo) Create(context.Context, *user.User) error { return nil }
func (f *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if f.exists[id] {
		return &user.User{ID: id}, nil
	}
	return nil, user.ErrNotFound
}
func (f *fakeUserRepo) GetByUsername(context.Context, string) (*user.User, error) {
	return nil, user.ErrNotFound
}

type fakeRevoker struct {
	revoked [][2]uuid.UUID
	err     error
}

func (f *fakeRevoker) PublishMemberRevoked(_ context.Context, conversationID, userID uuid.UUID) error {
	f.revoked = append(f.revoked, [2]uuid.UUID{conversationID, userID})
	return f.err
}

type postedEvent struct {
	ConvID uuid.UUID
	Actor  *uuid.UUID
	Event  domainmsg.SystemEvent
	Target uuid.UUID
	Detail string
}

type fakeSystemEventPoster struct{ events []postedEvent }

func (f *fakeSystemEventPoster) PostSystemEvent(_ context.Context, conv *domainconv.Conversation, actor *uuid.UUID, event domainmsg.SystemEvent, target uuid.UUID, detail string) error {
	f.events = append(f.events, postedEvent{conv.ID, actor, event, target, detail})
	return nil
}

func convMember(userID uuid.UUID, tier domainconv.AdminTier, creator bool) *domainconv.Member {
	return &domainconv.Member{UserID: userID, AdminTier: tier, IsCreator: creator, Role: domainconv.RoleMember}
}

func TestOpenDirect_CreatesWhenAbsent(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{other: true}}
	s := NewService(convs, users, &fakeRevoker{}, &fakeSystemEventPoster{})

	got, err := s.OpenDirect(context.Background(), me, other)
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if got.Kind != domainconv.KindDirect {
		t.Errorf("Kind = %q, want direct", got.Kind)
	}
	if len(convs.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(convs.created))
	}
}

func TestOpenDirect_ReturnsExisting(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	existing := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	convs := &fakeConvRepo{getDirect: func(_, _ uuid.UUID) (*domainconv.Conversation, error) { return existing, nil }}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{other: true}}
	s := NewService(convs, users, &fakeRevoker{}, &fakeSystemEventPoster{})

	got, err := s.OpenDirect(context.Background(), me, other)
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("ID = %v, want %v", got.ID, existing.ID)
	}
	if len(convs.created) != 0 {
		t.Errorf("Create called %d times, want 0", len(convs.created))
	}
}

func TestOpenDirect_Self(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.OpenDirect(context.Background(), me, me)
	if !errors.Is(err, ErrCannotDirectSelf) {
		t.Fatalf("err = %v, want ErrCannotDirectSelf", err)
	}
}

func TestOpenDirect_RecipientMissing(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{exists: map[uuid.UUID]bool{}}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.OpenDirect(context.Background(), me, other)
	if !errors.Is(err, ErrRecipientNotFound) {
		t.Fatalf("err = %v, want ErrRecipientNotFound", err)
	}
}

func TestOpenDirect_CreateRaceReReads(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	raced := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	calls := 0
	convs := &fakeConvRepo{
		createErr: domainconv.ErrDirectExists,
		getDirect: func(_, _ uuid.UUID) (*domainconv.Conversation, error) {
			calls++
			if calls == 1 {
				return nil, domainconv.ErrNotFound // first check: not there yet
			}
			return raced, nil // after the losing Create: the winner's row
		},
	}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{other: true}}
	s := NewService(convs, users, &fakeRevoker{}, &fakeSystemEventPoster{})

	got, err := s.OpenDirect(context.Background(), me, other)
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if got.ID != raced.ID {
		t.Errorf("ID = %v, want %v", got.ID, raced.ID)
	}
}

func TestCreateGroupChat_OK(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{}
	s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})

	got, err := s.CreateGroupChat(context.Background(), creator, "  Team  ")
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if got.Kind != domainconv.KindGroupChat || got.Title != "Team" {
		t.Errorf("got kind=%q title=%q", got.Kind, got.Title)
	}
	if len(convs.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(convs.created))
	}
}

func TestCreateGroupChat_BlankTitle(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{}
	s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})

	_, err := s.CreateGroupChat(context.Background(), creator, "   ")
	if !errors.Is(err, domainconv.ErrInvalidTitle) {
		t.Fatalf("err = %v, want ErrInvalidTitle", err)
	}
	if len(convs.created) != 0 {
		t.Errorf("Create called %d times, want 0", len(convs.created))
	}
}

func TestListMine_ClampsLimit(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	for _, tc := range []struct{ in, want int }{{0, DefaultListLimit}, {-5, DefaultListLimit}, {500, MaxListLimit}, {30, 30}} {
		convs := &fakeConvRepo{}
		s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
		if _, err := s.ListMine(context.Background(), me, time.Time{}, uuid.Nil, tc.in); err != nil {
			t.Fatalf("ListMine(%d): %v", tc.in, err)
		}
		if convs.listCalledWith.limit != tc.want {
			t.Errorf("limit %d -> repo got %d, want %d", tc.in, convs.listCalledWith.limit, tc.want)
		}
	}
}

func TestCreateChannel(t *testing.T) {
	creator := uuid.Must(uuid.NewV7())
	for _, kind := range []domainconv.Kind{domainconv.KindChannelGroup, domainconv.KindChannelBroadcast} {
		repo := &fakeConvRepo{}
		s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
		c, err := s.CreateChannel(context.Background(), creator, kind, "  News  ", true)
		if err != nil {
			t.Fatalf("CreateChannel(%s): %v", kind, err)
		}
		if c.Kind != kind || c.Title != "News" || !c.Public {
			t.Errorf("channel = %+v", c)
		}
		if len(repo.created) != 1 {
			t.Fatalf("Create called %d times, want 1", len(repo.created))
		}
	}
}

func TestCreateChannel_InvalidKind(t *testing.T) {
	repo := &fakeConvRepo{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.CreateChannel(context.Background(), uuid.Must(uuid.NewV7()), domainconv.KindGroupChat, "x", true)
	if !errors.Is(err, domainconv.ErrInvalidKind) {
		t.Fatalf("err = %v, want ErrInvalidKind", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("Create called despite a bad kind")
	}
}

func TestJoinChannel_GroupAddsMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: true},
		memberErr: domainconv.ErrNotMember,
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)

	got, err := s.JoinChannel(context.Background(), actor, chID)
	if err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if got.ID != chID {
		t.Errorf("returned conversation %v, want %v", got.ID, chID)
	}
	if len(repo.added) != 1 || repo.added[0].UserID != actor || repo.added[0].Role != domainconv.RoleMember {
		t.Fatalf("added = %+v, want one RoleMember for the actor", repo.added)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemMemberJoined ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor || poster.events[0].Target != actor {
		t.Fatalf("events = %+v, want one member_joined with actor==target==joiner", poster.events)
	}
}

func TestJoinChannel_BroadcastAddsSubscriber(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast, Public: true},
		memberErr: domainconv.ErrNotMember,
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})

	if _, err := s.JoinChannel(context.Background(), actor, chID); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if len(repo.added) != 1 || repo.added[0].Role != domainconv.RoleSubscriber {
		t.Fatalf("added = %+v, want one RoleSubscriber", repo.added)
	}
}

func TestJoinChannel_NotAChannel(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat, Public: true}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), chID)
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a non-channel's existence is not disclosed)", err)
	}
}

func TestJoinChannel_NotPublic(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: false}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), chID)
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a private channel is not disclosed)", err)
	}
}

func TestJoinChannel_AlreadyMemberIsIdempotent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: true},
		member: &domainconv.Member{ConversationID: chID, UserID: actor, Role: domainconv.RoleMember},
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})

	got, err := s.JoinChannel(context.Background(), actor, chID)
	if err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if got.ID != chID || len(repo.added) != 0 {
		t.Fatalf("expected the existing conversation and no AddMember, got %v / added=%+v", got.ID, repo.added)
	}
}

func TestJoinChannel_NotFound(t *testing.T) {
	repo := &fakeConvRepo{convErr: domainconv.ErrNotFound}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLeaveConversation_ChannelRemovesMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast, Public: true}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})

	if err := s.LeaveConversation(context.Background(), actor, chID); err != nil {
		t.Fatalf("LeaveConversation: %v", err)
	}
	if len(repo.removed) != 1 || repo.removed[0] != [2]uuid.UUID{chID, actor} {
		t.Fatalf("removed = %+v", repo.removed)
	}
}

func TestLeaveConversation_DirectNotAllowed(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindDirect}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if err := s.LeaveConversation(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a direct conversation cannot be left)", err)
	}
}

func TestLeaveConversation_NotMember(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: true},
		removeErr: domainconv.ErrNotMember,
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if err := s.LeaveConversation(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListPublicChannels_TrimsAndClamps(t *testing.T) {
	repo := &fakeConvRepo{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.ListPublicChannels(context.Background(), "  news  ", time.Time{}, uuid.Nil, 999); err != nil {
		t.Fatalf("ListPublicChannels: %v", err)
	}
	if repo.gotQuery != "news" {
		t.Errorf("query passed to repo = %q, want %q", repo.gotQuery, "news")
	}
	if repo.gotLimit != MaxListLimit {
		t.Errorf("limit passed to repo = %d, want %d", repo.gotLimit, MaxListLimit)
	}
}

func TestAddMember_AdminInvitesToGroupChat(t *testing.T) {
	actor, invitee := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			switch uid {
			case actor:
				return convMember(actor, domainconv.TierAdmin, false), nil
			case invitee:
				return convMember(invitee, domainconv.TierNone, false), nil
			}
			return nil, domainconv.ErrNotMember
		},
	}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{invitee: true}}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, users, &fakeRevoker{}, poster)

	m, err := s.AddMember(context.Background(), actor, chID, invitee)
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if len(repo.added) != 1 || repo.added[0].UserID != invitee || repo.added[0].Role != domainconv.RoleMember {
		t.Fatalf("added = %+v", repo.added)
	}
	if m == nil || m.UserID != invitee {
		t.Fatalf("returned member = %+v, want the invitee's row", m)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemMemberJoined ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor || poster.events[0].Target != invitee {
		t.Fatalf("events = %+v, want one member_joined with actor==inviter, target==invitee", poster.events)
	}
}

func TestAddMember_BroadcastInviteIsSubscriber(t *testing.T) {
	actor, invitee := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast},
		member: convMember(actor, domainconv.TierAdmin, false),
	}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{invitee: true}}
	s := NewService(repo, users, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.AddMember(context.Background(), actor, chID, invitee); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if repo.added[0].Role != domainconv.RoleSubscriber {
		t.Fatalf("invited role = %q, want subscriber", repo.added[0].Role)
	}
}

func TestAddMember_PlainMemberForbidden(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat},
		member: convMember(actor, domainconv.TierNone, false),
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.AddMember(context.Background(), actor, chID, uuid.Must(uuid.NewV7()))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if len(repo.added) != 0 {
		t.Error("AddMember should not have been called")
	}
}

func TestAddMember_InviteeMissing(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat},
		member: convMember(actor, domainconv.TierAdmin, false),
	}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{}} // the invitee does not resolve
	s := NewService(repo, users, &fakeRevoker{}, &fakeSystemEventPoster{})
	_, err := s.AddMember(context.Background(), actor, chID, uuid.Must(uuid.NewV7()))
	if !errors.Is(err, ErrInviteeNotFound) {
		t.Fatalf("err = %v, want ErrInviteeNotFound", err)
	}
}

func TestRemoveMember_SuperadminKicksAdminAndRevokes(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			switch uid {
			case actor:
				return convMember(actor, domainconv.TierSuperAdmin, false), nil
			case target:
				return convMember(target, domainconv.TierAdmin, false), nil
			}
			return nil, domainconv.ErrNotMember
		},
	}
	rev := &fakeRevoker{}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, rev, poster)

	if err := s.RemoveMember(context.Background(), actor, chID, target); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if len(repo.removed) != 1 || repo.removed[0] != [2]uuid.UUID{chID, target} {
		t.Fatalf("removed = %+v", repo.removed)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != [2]uuid.UUID{chID, target} {
		t.Fatalf("revoked = %+v, want one (chID, target)", rev.revoked)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemMemberRemoved ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor || poster.events[0].Target != target {
		t.Fatalf("events = %+v, want one member_removed with actor==kicker, target==target", poster.events)
	}
}

func TestRemoveMember_AdminCannotKickContentAdmin(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			if uid == actor {
				return convMember(actor, domainconv.TierAdmin, false), nil
			}
			return convMember(target, domainconv.TierContentAdmin, false), nil
		},
	}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if err := s.RemoveMember(context.Background(), actor, chID, target); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if len(repo.removed) != 0 || len(rev.revoked) != 0 {
		t.Fatal("nothing should have been removed or revoked")
	}
}

func TestRemoveMember_SelfIsRejected(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		member: convMember(actor, domainconv.TierSuperAdmin, false),
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if err := s.RemoveMember(context.Background(), actor, chID, actor); !errors.Is(err, ErrCannotKickSelf) {
		t.Fatalf("err = %v, want ErrCannotKickSelf", err)
	}
}

func TestSetMemberTier_SuperadminPromotesNoRevoke(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			if uid == actor {
				return convMember(actor, domainconv.TierSuperAdmin, false), nil
			}
			return convMember(target, domainconv.TierNone, false), nil
		},
	}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, target, domainconv.TierAdmin); err != nil {
		t.Fatalf("SetMemberTier: %v", err)
	}
	if len(repo.tierSet) != 1 || repo.tierSet[0].Tier != domainconv.TierAdmin || repo.tierSet[0].UserID != target {
		t.Fatalf("tierSet = %+v", repo.tierSet)
	}
	if len(rev.revoked) != 0 {
		t.Fatal("a tier change must not publish a revoke")
	}
}

func TestSetMemberTier_CreatorAdminMintsSuperadmin(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			if uid == actor {
				return convMember(actor, domainconv.TierCreatorAdmin, true), nil
			}
			return convMember(target, domainconv.TierNone, false), nil
		},
	}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, target, domainconv.TierSuperAdmin); err != nil {
		t.Fatalf("SetMemberTier: %v", err)
	}
	if len(repo.tierSet) != 1 || repo.tierSet[0].Tier != domainconv.TierSuperAdmin || repo.tierSet[0].UserID != target {
		t.Fatalf("tierSet = %+v", repo.tierSet)
	}
	if len(rev.revoked) != 0 {
		t.Fatal("a tier change must not publish a revoke")
	}
}

func TestSetMemberTier_SuperadminCannotMintSuperadmin(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			if uid == actor {
				return convMember(actor, domainconv.TierSuperAdmin, false), nil
			}
			return convMember(target, domainconv.TierNone, false), nil
		},
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, target, domainconv.TierSuperAdmin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestSetMemberTier_CreatorSelfRestore(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		member: convMember(actor, domainconv.TierNone, true),
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, actor, domainconv.TierCreatorAdmin); err != nil {
		t.Fatalf("creator self-restore: %v", err)
	}
	if len(repo.tierSet) != 1 || repo.tierSet[0].Tier != domainconv.TierCreatorAdmin {
		t.Fatalf("tierSet = %+v", repo.tierSet)
	}
}

func TestSetMemberTier_InvalidTier(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup},
		member: convMember(actor, domainconv.TierSuperAdmin, false),
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, uuid.Must(uuid.NewV7()), domainconv.AdminTier("mod")); !errors.Is(err, ErrInvalidTier) {
		t.Fatalf("err = %v, want ErrInvalidTier", err)
	}
}

func TestLeaveConversation_GroupChatPublishesRevoke(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat}}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if err := s.LeaveConversation(context.Background(), actor, chID); err != nil {
		t.Fatalf("LeaveConversation: %v", err)
	}
	if len(repo.removed) != 1 || repo.removed[0] != [2]uuid.UUID{chID, actor} {
		t.Fatalf("removed = %+v", repo.removed)
	}
	if len(rev.revoked) != 1 {
		t.Fatalf("revoked = %+v, want one", rev.revoked)
	}
}

func TestLeaveConversation_DirectRejected(t *testing.T) {
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if err := s.LeaveConversation(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetArchived_SuperadminArchives(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		member: &domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierSuperAdmin},
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	if err := s.SetArchived(context.Background(), actor, convID, true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	if len(repo.archivedSet) != 1 || repo.archivedSet[0] != (archivedCall{convID, true}) {
		t.Fatalf("archivedSet = %+v", repo.archivedSet)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemConversationArchived ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor ||
		poster.events[0].Target != uuid.Nil || poster.events[0].Detail != "" {
		t.Fatalf("events = %+v, want one conversation_archived (target uuid.Nil, detail \"\")", poster.events)
	}
}

func TestSetArchived_UnarchiveEmitsEvent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	now := time.Now()
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat, ArchivedAt: &now},
		member: &domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierSuperAdmin},
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	if err := s.SetArchived(context.Background(), actor, convID, false); err != nil {
		t.Fatalf("SetArchived(false): %v", err)
	}
	if len(repo.archivedSet) != 1 || repo.archivedSet[0] != (archivedCall{convID, false}) {
		t.Fatalf("archivedSet = %+v", repo.archivedSet)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemConversationUnarchived ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor ||
		poster.events[0].Target != uuid.Nil || poster.events[0].Detail != "" {
		t.Fatalf("events = %+v, want one conversation_unarchived", poster.events)
	}
}

func TestSetArchived_NoOpWhenAlreadyInState(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	now := time.Now()
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat, ArchivedAt: &now},
		member: &domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierSuperAdmin},
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	if err := s.SetArchived(context.Background(), actor, convID, true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	if len(repo.archivedSet) != 0 || len(poster.events) != 0 {
		t.Fatalf("archiving an already-archived conversation must be a no-op: archivedSet=%+v events=%+v", repo.archivedSet, poster.events)
	}
}

// The no-op early-return must run only after the actor is fully validated:
// a non-member still gets ErrNotMember and a non-superadmin member still gets
// ErrForbidden even when the conversation is already in the requested state.
func TestSetArchived_NoOpStillEnforcesAuthority(t *testing.T) {
	convID := uuid.Must(uuid.NewV7())
	now := time.Now()

	t.Run("non-member", func(t *testing.T) {
		repo := &fakeConvRepo{
			conv:      &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat, ArchivedAt: &now},
			memberErr: domainconv.ErrNotMember,
		}
		poster := &fakeSystemEventPoster{}
		s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
		if err := s.SetArchived(context.Background(), uuid.Must(uuid.NewV7()), convID, true); !errors.Is(err, domainconv.ErrNotMember) {
			t.Fatalf("err = %v, want ErrNotMember", err)
		}
		if len(repo.archivedSet) != 0 || len(poster.events) != 0 {
			t.Fatalf("archivedSet=%+v events=%+v, want both untouched", repo.archivedSet, poster.events)
		}
	})

	t.Run("non-superadmin member", func(t *testing.T) {
		actor := uuid.Must(uuid.NewV7())
		repo := &fakeConvRepo{
			conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat, ArchivedAt: &now},
			member: &domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierAdmin},
		}
		poster := &fakeSystemEventPoster{}
		s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
		if err := s.SetArchived(context.Background(), actor, convID, true); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
		if len(repo.archivedSet) != 0 || len(poster.events) != 0 {
			t.Fatalf("archivedSet=%+v events=%+v, want both untouched", repo.archivedSet, poster.events)
		}
	})
}

func TestSetArchived_AdminForbidden(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		member: &domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierAdmin},
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if err := s.SetArchived(context.Background(), actor, convID, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if len(repo.archivedSet) != 0 {
		t.Fatal("SetArchived should not have been called")
	}
}

func TestJoinChannel_ArchivedRejected(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	now := time.Now()
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: true, ArchivedAt: &now}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestAddMember_ArchivedRejected(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	now := time.Now()
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat, ArchivedAt: &now},
		member: &domainconv.Member{UserID: actor, AdminTier: domainconv.TierAdmin},
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.AddMember(context.Background(), actor, convID, uuid.Must(uuid.NewV7())); !errors.Is(err, domainconv.ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestLeaveConversation_UsesSuccession(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	called := false
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		succession: func(cid, uid uuid.UUID) (*domainconv.Member, error) {
			called = cid == convID && uid == actor
			return nil, nil
		},
	}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if err := s.LeaveConversation(context.Background(), actor, convID); err != nil {
		t.Fatalf("LeaveConversation: %v", err)
	}
	if !called {
		t.Fatal("LeaveConversation must call RemoveMemberWithSuccession")
	}
	if len(rev.revoked) != 1 {
		t.Fatalf("revoked = %+v, want one", rev.revoked)
	}
}

func TestClaimCreator(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		claim: func(cid, uid uuid.UUID) (*domainconv.Member, error) {
			return &domainconv.Member{UserID: uid, IsCreator: true, AdminTier: domainconv.TierCreatorAdmin}, nil
		},
	}
	repo.member = convMember(actor, domainconv.TierAdmin, false) // membership gate must pass
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	m, err := s.ClaimCreator(context.Background(), actor, convID)
	if err != nil {
		t.Fatalf("ClaimCreator: %v", err)
	}
	if m.UserID != actor || !m.IsCreator {
		t.Fatalf("m = %+v", m)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemMemberTierChanged ||
		poster.events[0].Actor == nil || *poster.events[0].Actor != actor || poster.events[0].Target != actor ||
		poster.events[0].Detail != string(domainconv.TierCreatorAdmin) {
		t.Fatalf("events = %+v, want one member_tier_changed with actor==target==claimer, detail creator_admin", poster.events)
	}
}

func TestClaimCreator_SeatTaken(t *testing.T) {
	convID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		member: convMember(actor, domainconv.TierAdmin, false),
		claim:  func(_, _ uuid.UUID) (*domainconv.Member, error) { return nil, domainconv.ErrCreatorSeatTaken },
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.ClaimCreator(context.Background(), actor, convID); !errors.Is(err, domainconv.ErrCreatorSeatTaken) {
		t.Fatalf("err = %v, want ErrCreatorSeatTaken", err)
	}
}

func TestClaimCreator_DirectRejected(t *testing.T) {
	convID := uuid.Must(uuid.NewV7())
	claimCalled := false
	repo := &fakeConvRepo{
		conv:  &domainconv.Conversation{ID: convID, Kind: domainconv.KindDirect},
		claim: func(_, _ uuid.UUID) (*domainconv.Member, error) { claimCalled = true; return nil, nil },
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.ClaimCreator(context.Background(), uuid.Must(uuid.NewV7()), convID); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a DM is never claimable)", err)
	}
	if claimCalled {
		t.Fatal("repo.ClaimCreator must not be reached for a direct conversation")
	}
}

func TestClaimCreator_NonMember(t *testing.T) {
	convID := uuid.Must(uuid.NewV7())
	claimCalled := false
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		memberErr: domainconv.ErrNotMember,
		claim:     func(_, _ uuid.UUID) (*domainconv.Member, error) { claimCalled = true; return nil, nil },
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.ClaimCreator(context.Background(), uuid.Must(uuid.NewV7()), convID); !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
	if claimCalled {
		t.Fatal("repo.ClaimCreator must not be reached for a non-member")
	}
}

func TestRemoveMember_DirectRejected(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: convID, Kind: domainconv.KindDirect},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			return convMember(uid, domainconv.TierCreatorAdmin, uid == actor), nil
		},
	}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev, &fakeSystemEventPoster{})
	if err := s.RemoveMember(context.Background(), actor, convID, target); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a DM participant is never kickable)", err)
	}
	if len(repo.removed) != 0 || len(rev.revoked) != 0 {
		t.Fatalf("nothing should have been removed or revoked: removed=%+v revoked=%+v", repo.removed, rev.revoked)
	}
}

func TestListMembers(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:       &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		member:     &domainconv.Member{UserID: actor},
		memberList: []*domainconv.MemberProfile{{Member: domainconv.Member{UserID: actor}, Username: "u"}},
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	got, err := s.ListMembers(context.Background(), actor, convID, true, time.Time{}, uuid.Nil, 999)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(got) != 1 || !repo.gotAdminsOnly {
		t.Fatalf("got %d rows, adminsOnly=%v", len(got), repo.gotAdminsOnly)
	}
}

func TestListMembers_NonMember(t *testing.T) {
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat},
		memberErr: domainconv.ErrNotMember,
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, &fakeSystemEventPoster{})
	if _, err := s.ListMembers(context.Background(), uuid.Must(uuid.NewV7()), repo.conv.ID, false, time.Time{}, uuid.Nil, 20); !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestSetMemberTier_EmitsTierChangedEvent(t *testing.T) {
	actor, target := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv: &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		getMemberFunc: func(_ context.Context, _ uuid.UUID, uid uuid.UUID) (*domainconv.Member, error) {
			if uid == actor {
				return &domainconv.Member{UserID: actor, AdminTier: domainconv.TierSuperAdmin}, nil
			}
			return &domainconv.Member{UserID: target, AdminTier: domainconv.TierNone}, nil
		},
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	if _, err := s.SetMemberTier(context.Background(), actor, convID, target, domainconv.TierAdmin); err != nil {
		t.Fatalf("SetMemberTier: %v", err)
	}
	if len(poster.events) != 1 || poster.events[0].Event != domainmsg.SystemMemberTierChanged || poster.events[0].Detail != "admin" || poster.events[0].Target != target {
		t.Fatalf("events = %+v", poster.events)
	}
}

func TestLeaveConversation_EmitsLeftAndSuccession(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	heir := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:       &domainconv.Conversation{ID: convID, Kind: domainconv.KindGroupChat},
		succession: func(_, _ uuid.UUID) (*domainconv.Member, error) { return &domainconv.Member{UserID: heir}, nil },
	}
	poster := &fakeSystemEventPoster{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{}, poster)
	if err := s.LeaveConversation(context.Background(), actor, convID); err != nil {
		t.Fatalf("LeaveConversation: %v", err)
	}
	if len(poster.events) != 2 {
		t.Fatalf("events = %+v, want member_left + member_tier_changed", poster.events)
	}
	if poster.events[0].Event != domainmsg.SystemMemberLeft || poster.events[1].Event != domainmsg.SystemMemberTierChanged || poster.events[1].Actor != nil {
		t.Fatalf("events = %+v", poster.events)
	}
}
