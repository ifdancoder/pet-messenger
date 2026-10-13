package query_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
	domainuser "pet-messenger/internal/user/domain"
)

type fakePublicChannelView struct {
	channels []*domain.Conversation
	lastArgs struct {
		query string
		limit int
	}
}

func (f *fakePublicChannelView) ListPublicChannels(_ context.Context, query string, _ time.Time, _ uuid.UUID, limit int) ([]*domain.Conversation, error) {
	f.lastArgs.query, f.lastArgs.limit = query, limit
	return f.channels, nil
}

type fakeMemberView struct {
	members []*domain.Member
}

func (f *fakeMemberView) ListMembers(_ context.Context, _ uuid.UUID, _ bool, _ time.Time, _ uuid.UUID, _ int) ([]*domain.Member, error) {
	return f.members, nil
}

// fakeProfileView is the minimal out.ProfileBatchViewPort the ListMembers
// query needs to zip display fields onto the bare roster rows returned by
// fakeMemberView.
type fakeProfileView struct {
	profiles []*domainuser.Profile
}

func (f *fakeProfileView) GetProfilesByIDs(_ context.Context, _ []uuid.UUID) ([]*domainuser.Profile, error) {
	return f.profiles, nil
}

type fakeInboxView struct {
	entries []*out.InboxEntry
}

func (f *fakeInboxView) ListInbox(_ context.Context, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]*out.InboxEntry, error) {
	return f.entries, nil
}

// fakeLoaderForQuery is the minimal out.LoadConversationPort the
// ListMembers query needs (GetByID + GetMember for the actor-is-a-member
// gate).
type fakeLoaderForQuery struct {
	conv    *domain.Conversation
	members map[uuid.UUID]*domain.Member
}

func (f *fakeLoaderForQuery) GetByID(_ context.Context, id uuid.UUID) (*domain.Conversation, error) {
	if f.conv != nil && f.conv.ID == id {
		return f.conv, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeLoaderForQuery) GetMember(_ context.Context, _, userID uuid.UUID) (*domain.Member, error) {
	if m, ok := f.members[userID]; ok {
		return m, nil
	}
	return nil, domain.ErrNotMember
}

func (f *fakeLoaderForQuery) GetDirect(context.Context, uuid.UUID, uuid.UUID) (*domain.Conversation, error) {
	return nil, domain.ErrNotFound
}
