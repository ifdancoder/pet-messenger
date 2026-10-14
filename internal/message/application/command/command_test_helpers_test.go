// internal/message/application/command/command_test_helpers_test.go
package command

import (
	"context"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

type fakeMessageLoader struct {
	byID  map[uuid.UUID]*domainmsg.Message
	byKey map[string]*domainmsg.Message
}

func (f *fakeMessageLoader) GetByID(_ context.Context, id uuid.UUID) (*domainmsg.Message, error) {
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return nil, domainmsg.ErrNotFound
}

func (f *fakeMessageLoader) GetByIdempotencyKey(_ context.Context, _ uuid.UUID, _ uuid.UUID, key string) (*domainmsg.Message, error) {
	if m, ok := f.byKey[key]; ok {
		return m, nil
	}
	return nil, domainmsg.ErrNotFound
}

type fakeMessageWriter struct {
	created   []*domainmsg.Message
	createErr error
	updated   []*domainmsg.Message
	updateErr error
}

func (f *fakeMessageWriter) Create(_ context.Context, m *domainmsg.Message) error {
	if f.createErr != nil {
		return f.createErr
	}
	m.Seq = int64(len(f.created) + 1)
	f.created = append(f.created, m)
	return nil
}

func (f *fakeMessageWriter) Update(_ context.Context, m *domainmsg.Message) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, m)
	return nil
}

type fakeConversationMembership struct {
	conv      *domainconv.Conversation
	convErr   error
	member    *domainconv.Member
	memberErr error
}

func (f *fakeConversationMembership) GetByID(context.Context, uuid.UUID) (*domainconv.Conversation, error) {
	return f.conv, f.convErr
}

func (f *fakeConversationMembership) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return f.member, f.memberErr
}

type fakeReadCursor struct {
	readCursor [3]uuid.UUID
}

func (f *fakeReadCursor) SetLastReadMessage(_ context.Context, conversationID, userID, messageID uuid.UUID) error {
	f.readCursor = [3]uuid.UUID{conversationID, userID, messageID}
	return nil
}

type fakePublisher struct {
	created []*domainmsg.Message
	updated []*domainmsg.Message
	deleted []*domainmsg.Message
	err     error
}

func (f *fakePublisher) PublishCreated(_ context.Context, m *domainmsg.Message) error {
	f.created = append(f.created, m)
	return f.err
}
func (f *fakePublisher) PublishUpdated(_ context.Context, m *domainmsg.Message) error {
	f.updated = append(f.updated, m)
	return f.err
}
func (f *fakePublisher) PublishDeleted(_ context.Context, m *domainmsg.Message) error {
	f.deleted = append(f.deleted, m)
	return f.err
}

type fakeNotifProducer struct{ messageSent, membershipEvent int }

func (f *fakeNotifProducer) MessageSent(context.Context, *domainmsg.Message, *domainconv.Conversation) error {
	f.messageSent++
	return nil
}
func (f *fakeNotifProducer) MembershipEvent(context.Context, domainmsg.SystemEvent, uuid.UUID, uuid.UUID, string) error {
	f.membershipEvent++
	return nil
}

// groupChatWithMember returns a fakeConversationMembership whose
// conversation is a group chat the actor is a plain member of (so CanPost
// is true).
func groupChatWithMember() *fakeConversationMembership {
	return &fakeConversationMembership{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat},
		member: &domainconv.Member{Role: domainconv.RoleMember},
	}
}
