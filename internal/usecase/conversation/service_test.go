package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
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

func convMember(userID uuid.UUID, tier domainconv.AdminTier, creator bool) *domainconv.Member {
	return &domainconv.Member{UserID: userID, AdminTier: tier, IsCreator: creator, Role: domainconv.RoleMember}
}

func TestOpenDirect_CreatesWhenAbsent(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{other: true}}
	s := NewService(convs, users, &fakeRevoker{})

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
	s := NewService(convs, users, &fakeRevoker{})

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
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{}, &fakeRevoker{})
	_, err := s.OpenDirect(context.Background(), me, me)
	if !errors.Is(err, ErrCannotDirectSelf) {
		t.Fatalf("err = %v, want ErrCannotDirectSelf", err)
	}
}

func TestOpenDirect_RecipientMissing(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{exists: map[uuid.UUID]bool{}}, &fakeRevoker{})
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
	s := NewService(convs, users, &fakeRevoker{})

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
	s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{})

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
	s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{})

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
		s := NewService(convs, &fakeUserRepo{}, &fakeRevoker{})
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
		s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})

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
}

func TestJoinChannel_BroadcastAddsSubscriber(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast, Public: true},
		memberErr: domainconv.ErrNotMember,
	}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})

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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), chID)
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a non-channel's existence is not disclosed)", err)
	}
}

func TestJoinChannel_NotPublic(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: false}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})

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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLeaveConversation_ChannelRemovesMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast, Public: true}}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})

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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
	if err := s.LeaveConversation(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListPublicChannels_TrimsAndClamps(t *testing.T) {
	repo := &fakeConvRepo{}
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, users, &fakeRevoker{})

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
}

func TestAddMember_BroadcastInviteIsSubscriber(t *testing.T) {
	actor, invitee := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast},
		member: convMember(actor, domainconv.TierAdmin, false),
	}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{invitee: true}}
	s := NewService(repo, users, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, users, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, rev)

	if err := s.RemoveMember(context.Background(), actor, chID, target); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if len(repo.removed) != 1 || repo.removed[0] != [2]uuid.UUID{chID, target} {
		t.Fatalf("removed = %+v", repo.removed)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != [2]uuid.UUID{chID, target} {
		t.Fatalf("revoked = %+v, want one (chID, target)", rev.revoked)
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
	s := NewService(repo, &fakeUserRepo{}, rev)
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, rev)
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
	s := NewService(repo, &fakeUserRepo{}, rev)
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
	if _, err := s.SetMemberTier(context.Background(), actor, chID, uuid.Must(uuid.NewV7()), domainconv.AdminTier("mod")); !errors.Is(err, ErrInvalidTier) {
		t.Fatalf("err = %v, want ErrInvalidTier", err)
	}
}

func TestLeaveConversation_GroupChatPublishesRevoke(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindGroupChat}}
	rev := &fakeRevoker{}
	s := NewService(repo, &fakeUserRepo{}, rev)
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
	s := NewService(repo, &fakeUserRepo{}, &fakeRevoker{})
	if err := s.LeaveConversation(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
