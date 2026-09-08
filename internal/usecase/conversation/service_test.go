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
}

func (f *fakeConvRepo) Create(_ context.Context, c *domainconv.Conversation, _ []*domainconv.Member) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, c)
	return nil
}
func (f *fakeConvRepo) GetByID(context.Context, uuid.UUID) (*domainconv.Conversation, error) {
	return nil, domainconv.ErrNotFound
}
func (f *fakeConvRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
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
func (f *fakeConvRepo) AddMember(context.Context, *domainconv.Member) error {
	return nil
}
func (f *fakeConvRepo) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (f *fakeConvRepo) ListPublicChannels(context.Context, string, time.Time, uuid.UUID, int) ([]*domainconv.Conversation, error) {
	return nil, nil
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
