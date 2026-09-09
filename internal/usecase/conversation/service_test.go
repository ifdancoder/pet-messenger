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

	conv      *domainconv.Conversation
	convErr   error
	member    *domainconv.Member
	memberErr error

	added      []*domainconv.Member
	addErr     error
	removed    [][2]uuid.UUID
	removeErr  error
	publicList []*domainconv.Conversation
	gotQuery   string
	gotLimit   int

	tierSet []tierSetCall
	tierErr error
}

type tierSetCall struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	Tier           domainconv.AdminTier
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
func (f *fakeConvRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
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

func TestOpenDirect_CreatesWhenAbsent(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{}
	users := &fakeUserRepo{exists: map[uuid.UUID]bool{other: true}}
	s := NewService(convs, users)

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
	s := NewService(convs, users)

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
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{})
	_, err := s.OpenDirect(context.Background(), me, me)
	if !errors.Is(err, ErrCannotDirectSelf) {
		t.Fatalf("err = %v, want ErrCannotDirectSelf", err)
	}
}

func TestOpenDirect_RecipientMissing(t *testing.T) {
	me, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	s := NewService(&fakeConvRepo{}, &fakeUserRepo{exists: map[uuid.UUID]bool{}})
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
	s := NewService(convs, users)

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
	s := NewService(convs, &fakeUserRepo{})

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
	s := NewService(convs, &fakeUserRepo{})

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
		s := NewService(convs, &fakeUserRepo{})
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
		s := NewService(repo, &fakeUserRepo{})
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
	s := NewService(repo, &fakeUserRepo{})
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
	s := NewService(repo, &fakeUserRepo{})

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
	s := NewService(repo, &fakeUserRepo{})

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
	s := NewService(repo, &fakeUserRepo{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), chID)
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a non-channel's existence is not disclosed)", err)
	}
}

func TestJoinChannel_NotPublic(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: false}}
	s := NewService(repo, &fakeUserRepo{})
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
	s := NewService(repo, &fakeUserRepo{})

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
	s := NewService(repo, &fakeUserRepo{})
	_, err := s.JoinChannel(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLeaveChannel(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelBroadcast, Public: true}}
	s := NewService(repo, &fakeUserRepo{})

	if err := s.LeaveChannel(context.Background(), actor, chID); err != nil {
		t.Fatalf("LeaveChannel: %v", err)
	}
	if len(repo.removed) != 1 || repo.removed[0] != [2]uuid.UUID{chID, actor} {
		t.Fatalf("removed = %+v", repo.removed)
	}
}

func TestLeaveChannel_NotAChannel(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{conv: &domainconv.Conversation{ID: chID, Kind: domainconv.KindDirect}}
	s := NewService(repo, &fakeUserRepo{})
	if err := s.LeaveChannel(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (a non-channel's existence is not disclosed)", err)
	}
}

func TestLeaveChannel_NotMember(t *testing.T) {
	chID := uuid.Must(uuid.NewV7())
	repo := &fakeConvRepo{
		conv:      &domainconv.Conversation{ID: chID, Kind: domainconv.KindChannelGroup, Public: true},
		removeErr: domainconv.ErrNotMember,
	}
	s := NewService(repo, &fakeUserRepo{})
	if err := s.LeaveChannel(context.Background(), uuid.Must(uuid.NewV7()), chID); !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListPublicChannels_TrimsAndClamps(t *testing.T) {
	repo := &fakeConvRepo{}
	s := NewService(repo, &fakeUserRepo{})
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
