package command_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
	"pet-messenger/internal/domain/user"
)

// fakeLoader is an in-memory out.LoadConversationPort.
type fakeLoader struct {
	byID       map[uuid.UUID]*domain.Conversation
	members    map[uuid.UUID]map[uuid.UUID]*domain.Member // conversationID -> userID -> member
	direct     map[[2]uuid.UUID]*domain.Conversation
	getByIDErr error
}

func newFakeLoader() *fakeLoader {
	return &fakeLoader{
		byID:    map[uuid.UUID]*domain.Conversation{},
		members: map[uuid.UUID]map[uuid.UUID]*domain.Member{},
		direct:  map[[2]uuid.UUID]*domain.Conversation{},
	}
}

func directKey(a, b uuid.UUID) [2]uuid.UUID {
	if a.String() > b.String() {
		a, b = b, a
	}
	return [2]uuid.UUID{a, b}
}

func (f *fakeLoader) put(c *domain.Conversation, members ...*domain.Member) {
	f.byID[c.ID] = c
	if f.members[c.ID] == nil {
		f.members[c.ID] = map[uuid.UUID]*domain.Member{}
	}
	for _, m := range members {
		f.members[c.ID][m.UserID] = m
	}
	if c.Kind == domain.KindDirect && len(members) == 2 {
		f.direct[directKey(members[0].UserID, members[1].UserID)] = c
	}
}

func (f *fakeLoader) GetByID(_ context.Context, id uuid.UUID) (*domain.Conversation, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeLoader) GetMember(_ context.Context, conversationID, userID uuid.UUID) (*domain.Member, error) {
	if m, ok := f.members[conversationID][userID]; ok {
		return m, nil
	}
	return nil, domain.ErrNotMember
}

func (f *fakeLoader) GetDirect(_ context.Context, userA, userB uuid.UUID) (*domain.Conversation, error) {
	if c, ok := f.direct[directKey(userA, userB)]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

// fakeWriter is an in-memory out.UpdateConversationPort backed by
// fakeLoader's maps, so a handler under test sees its own writes on the
// next read.
type fakeWriter struct {
	loader       *fakeLoader
	createErr    error
	directExists bool // next Create for a direct pair returns domain.ErrDirectExists
}

func (f *fakeWriter) Create(_ context.Context, c *domain.Conversation, members []*domain.Member) error {
	if f.createErr != nil {
		return f.createErr
	}
	if c.Kind == domain.KindDirect && f.directExists {
		return domain.ErrDirectExists
	}
	f.loader.put(c, members...)
	return nil
}

func (f *fakeWriter) AddMember(_ context.Context, m *domain.Member) error {
	if f.loader.members[m.ConversationID] == nil {
		f.loader.members[m.ConversationID] = map[uuid.UUID]*domain.Member{}
	}
	f.loader.members[m.ConversationID][m.UserID] = m
	return nil
}

func (f *fakeWriter) RemoveMember(_ context.Context, conversationID, userID uuid.UUID) error {
	if _, ok := f.loader.members[conversationID][userID]; !ok {
		return domain.ErrNotMember
	}
	delete(f.loader.members[conversationID], userID)
	return nil
}

func (f *fakeWriter) SetMemberTier(_ context.Context, conversationID, userID uuid.UUID, tier domain.AdminTier) error {
	m, ok := f.loader.members[conversationID][userID]
	if !ok {
		return domain.ErrNotMember
	}
	m.AdminTier = tier
	return nil
}

func (f *fakeWriter) SetArchived(_ context.Context, conversationID uuid.UUID, archived bool) error {
	c, ok := f.loader.byID[conversationID]
	if !ok {
		return domain.ErrNotFound
	}
	if archived {
		now := time.Now().UTC()
		c.ArchivedAt = &now
	} else {
		c.ArchivedAt = nil
	}
	return nil
}

func (f *fakeWriter) RemoveMemberWithSuccession(_ context.Context, conversationID, userID uuid.UUID) (*domain.Member, error) {
	m, ok := f.loader.members[conversationID][userID]
	if !ok {
		return nil, domain.ErrNotMember
	}
	wasCreator := m.IsCreator
	delete(f.loader.members[conversationID], userID)
	if !wasCreator {
		return nil, nil
	}
	var heir *domain.Member
	for _, cand := range f.loader.members[conversationID] {
		if heir == nil || cand.JoinedAt.Before(heir.JoinedAt) {
			heir = cand
		}
	}
	if heir == nil {
		return nil, nil
	}
	heir.IsCreator = true
	heir.AdminTier = domain.TierCreatorAdmin
	return heir, nil
}

func (f *fakeWriter) ClaimCreator(_ context.Context, conversationID, userID uuid.UUID) (*domain.Member, error) {
	for _, m := range f.loader.members[conversationID] {
		if m.IsCreator {
			return nil, domain.ErrCreatorSeatTaken
		}
	}
	m, ok := f.loader.members[conversationID][userID]
	if !ok {
		return nil, domain.ErrNotMember
	}
	m.IsCreator = true
	m.AdminTier = domain.TierCreatorAdmin
	return m, nil
}

// fakePublisher is an in-memory out.EventPublisherPort that records every
// published event for assertions.
type fakePublisher struct {
	events []domain.Event
}

func (f *fakePublisher) Publish(_ context.Context, event domain.Event) error {
	f.events = append(f.events, event)
	return nil
}

// fakeUsers is an in-memory user.Repository stand-in — only GetByID is
// exercised by this package's handlers.
type fakeUsers struct {
	exists map[uuid.UUID]bool
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if f.exists[id] {
		return &user.User{ID: id}, nil
	}
	return nil, user.ErrNotFound
}

// Create, GetByUsername, and Update exist only to satisfy user.Repository
// — this package's handlers never call them.
func (f *fakeUsers) Create(context.Context, *user.User) error { return nil }
func (f *fakeUsers) GetByUsername(context.Context, string) (*user.User, error) {
	return nil, user.ErrNotFound
}
func (f *fakeUsers) Update(context.Context, *user.User) error { return nil }

// fakeSystemEventPoster is an in-memory out.SystemEventPoster that counts
// every post for assertions.
type fakeSystemEventPoster struct {
	posts int
}

func (f *fakeSystemEventPoster) PostSystemEvent(_ context.Context, _ *domain.Conversation, _ *uuid.UUID, _ domainmsg.SystemEvent, _ uuid.UUID, _ string) error {
	f.posts++
	return nil
}
