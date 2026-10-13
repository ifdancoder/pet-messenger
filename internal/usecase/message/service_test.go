package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/domain/message"
)

// --- fakes -----------------------------------------------------------------

type fakeMsgRepo struct {
	created   []*domainmsg.Message
	createErr error
	byID      map[uuid.UUID]*domainmsg.Message
	byKey     map[string]*domainmsg.Message
	updated   []*domainmsg.Message
	updateErr error

	listByConvCalled bool
	listBeforeCalled bool
	gotAfterSeq      int64
	gotBeforeSeq     int64
	gotLimit         int
	listResult       []*domainmsg.Message
}

func (f *fakeMsgRepo) Create(_ context.Context, m *domainmsg.Message) error {
	if f.createErr != nil {
		return f.createErr
	}
	m.Seq = int64(len(f.created) + 1)
	f.created = append(f.created, m)
	return nil
}
func (f *fakeMsgRepo) GetByID(_ context.Context, id uuid.UUID) (*domainmsg.Message, error) {
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return nil, domainmsg.ErrNotFound
}
func (f *fakeMsgRepo) GetByIdempotencyKey(_ context.Context, _ uuid.UUID, _ uuid.UUID, key string) (*domainmsg.Message, error) {
	if m, ok := f.byKey[key]; ok {
		return m, nil
	}
	return nil, domainmsg.ErrNotFound
}
func (f *fakeMsgRepo) ListByConversation(_ context.Context, _ uuid.UUID, afterSeq int64, limit int) ([]*domainmsg.Message, error) {
	f.listByConvCalled, f.gotAfterSeq, f.gotLimit = true, afterSeq, limit
	return f.listResult, nil
}
func (f *fakeMsgRepo) ListBeforeSeq(_ context.Context, _ uuid.UUID, beforeSeq int64, limit int) ([]*domainmsg.Message, error) {
	f.listBeforeCalled, f.gotBeforeSeq, f.gotLimit = true, beforeSeq, limit
	return f.listResult, nil
}
func (f *fakeMsgRepo) Update(_ context.Context, m *domainmsg.Message) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, m)
	return nil
}

type fakeConvRepo struct {
	conv       *domainconv.Conversation
	convErr    error
	member     *domainconv.Member
	memberErr  error
	readCursor [3]uuid.UUID
}

func (f *fakeConvRepo) Create(context.Context, *domainconv.Conversation, []*domainconv.Member) error {
	return nil
}
func (f *fakeConvRepo) GetByID(context.Context, uuid.UUID) (*domainconv.Conversation, error) {
	return f.conv, f.convErr
}
func (f *fakeConvRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return f.member, f.memberErr
}
func (f *fakeConvRepo) GetDirect(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) {
	return nil, domainconv.ErrNotFound
}
func (f *fakeConvRepo) ListForUser(context.Context, uuid.UUID, time.Time, uuid.UUID, int) ([]*domainconv.Conversation, error) {
	return nil, nil
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
func (f *fakeConvRepo) SetMemberTier(context.Context, uuid.UUID, uuid.UUID, domainconv.AdminTier) error {
	return nil
}
func (f *fakeConvRepo) SetLastReadMessage(_ context.Context, conversationID, userID, messageID uuid.UUID) error {
	f.readCursor = [3]uuid.UUID{conversationID, userID, messageID}
	return nil
}
func (f *fakeConvRepo) SetArchived(context.Context, uuid.UUID, bool) error { return nil }
func (f *fakeConvRepo) RemoveMemberWithSuccession(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return nil, nil
}
func (f *fakeConvRepo) ClaimCreator(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return nil, nil
}
func (f *fakeConvRepo) ListMembers(context.Context, uuid.UUID, bool, time.Time, uuid.UUID, int) ([]*domainconv.Member, error) {
	return nil, nil
}
func (f *fakeConvRepo) ListMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
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

// newSvc builds a Service with a capturing publisher and a no-op notification
// producer.
func newSvc(msgs domainmsg.Repository, convs ConversationReader) (*Service, *fakePublisher) {
	pub := &fakePublisher{}
	return NewService(msgs, convs, pub, &fakeNotifProducer{}), pub
}

// groupChatWithMember returns a fakeConvRepo whose conversation is a group chat
// the actor is a plain member of (so CanPost is true).
func groupChatWithMember() *fakeConvRepo {
	return &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat},
		member: &domainconv.Member{Role: domainconv.RoleMember},
	}
}

// --- SendMessage ---------------------------------------------------------

func TestSendMessage_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	msgs := &fakeMsgRepo{}
	notifProd := &fakeNotifProducer{}
	s := NewService(msgs, convs, &fakePublisher{}, notifProd)

	m, err := s.SendMessage(context.Background(), actor, convID, "  hi  ", nil, "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(msgs.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(msgs.created))
	}
	if m.Content != "hi" || m.SenderID == nil || *m.SenderID != actor {
		t.Errorf("message = %+v", m)
	}
	if notifProd.messageSent != 1 {
		t.Errorf("notifProd.messageSent = %d, want 1", notifProd.messageSent)
	}
}

func TestSendMessage_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, "")
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestSendMessage_ConversationNotFound(t *testing.T) {
	convs := &fakeConvRepo{convErr: domainconv.ErrNotFound}
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "hi", nil, "")
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSendMessage_CannotPost(t *testing.T) {
	convs := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelBroadcast},
		member: &domainconv.Member{Role: domainconv.RoleSubscriber},
	}
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, "")
	if !errors.Is(err, ErrCannotPost) {
		t.Fatalf("err = %v, want ErrCannotPost", err)
	}
}

func TestSendMessage_ReplyTargetMissing(t *testing.T) {
	convs := groupChatWithMember()
	s, _ := newSvc(&fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{}}, convs)
	missing := uuid.Must(uuid.NewV7())
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", &missing, "")
	if !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("err = %v, want ErrReplyNotFound", err)
	}
}

func TestSendMessage_ReplyTargetOtherConversation(t *testing.T) {
	convs := groupChatWithMember()
	other := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7())}
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{other.ID: other}}
	s, _ := newSvc(msgs, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", &other.ID, "")
	if !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("err = %v, want ErrReplyNotFound", err)
	}
}

func TestSendMessage_IdempotentReplay(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	existing := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: 7, Content: "hi"}
	msgs := &fakeMsgRepo{
		createErr: domainmsg.ErrDuplicateIdempotencyKey,
		byKey:     map[string]*domainmsg.Message{"k1": existing},
	}
	s, _ := newSvc(msgs, convs)

	m, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convID, "hi", nil, "k1")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if m.ID != existing.ID || m.Seq != 7 {
		t.Errorf("got id=%v seq=%d, want the existing id=%v seq=7", m.ID, m.Seq, existing.ID)
	}
}

func TestSendMessage_ArchivedRejected(t *testing.T) {
	now := time.Now()
	convs := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat, ArchivedAt: &now},
		member: &domainconv.Member{Role: domainconv.RoleMember},
	}
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	if _, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, ""); !errors.Is(err, ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	convs := groupChatWithMember()
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "   ", nil, "")
	if !errors.Is(err, domainmsg.ErrEmptyContent) {
		t.Fatalf("err = %v, want ErrEmptyContent", err)
	}
}

// --- EditMessage -------------------------------------------------------------

func ownText(t *testing.T, actor, convID uuid.UUID) *domainmsg.Message {
	t.Helper()
	m, err := domainmsg.NewText(convID, actor, "original", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	return m
}

func TestEditMessage_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	got, err := s.EditMessage(context.Background(), actor, convID, m.ID, "  edited  ")
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if got.Content != "edited" || !got.IsEdited() {
		t.Errorf("got %+v", got)
	}
	if len(msgs.updated) != 1 {
		t.Fatalf("Update called %d times, want 1", len(msgs.updated))
	}
}

func TestEditMessage_NotAuthor(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, uuid.Must(uuid.NewV7()), convID) // authored by someone else
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	_, err := s.EditMessage(context.Background(), uuid.Must(uuid.NewV7()), convID, m.ID, "x")
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
	if len(msgs.updated) != 0 {
		t.Errorf("Update called %d times, want 0", len(msgs.updated))
	}
}

func TestEditMessage_WrongConversation(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, uuid.Must(uuid.NewV7())) // belongs to another conversation
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	_, err := s.EditMessage(context.Background(), actor, convs.conv.ID, m.ID, "x")
	if !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEditMessage_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.EditMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, uuid.Must(uuid.NewV7()), "x")
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestEditMessage_DeletedIsNotEditable(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	m.MarkDeleted(time.Now().UTC())
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	_, err := s.EditMessage(context.Background(), actor, convID, m.ID, "x")
	if !errors.Is(err, domainmsg.ErrNotEditable) {
		t.Fatalf("err = %v, want ErrNotEditable", err)
	}
}

// --- DeleteMessage ---------------------------------------------------------

func TestDeleteMessage_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	got, err := s.DeleteMessage(context.Background(), actor, convID, m.ID)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if !got.IsDeleted() || got.Content != "" {
		t.Errorf("got %+v, want tombstoned", got)
	}
	if len(msgs.updated) != 1 {
		t.Fatalf("Update called %d times, want 1", len(msgs.updated))
	}
}

func TestDeleteMessage_NotAuthor(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, uuid.Must(uuid.NewV7()), convID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	_, err := s.DeleteMessage(context.Background(), uuid.Must(uuid.NewV7()), convID, m.ID)
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
	if len(msgs.updated) != 0 {
		t.Errorf("Update called %d times, want 0", len(msgs.updated))
	}
}

func TestDeleteMessage_ContentAdminDeletesAnothersMessage(t *testing.T) {
	author := uuid.Must(uuid.NewV7())
	moderator := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())

	convs := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindChannelGroup},
		member: &domainconv.Member{UserID: moderator, Role: domainconv.RoleMember, AdminTier: domainconv.TierContentAdmin},
	}
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{
		msgID: {ID: msgID, ConversationID: convID, SenderID: &author, Type: domainmsg.TypeText, Content: "x"},
	}}
	s, pub := newSvc(msgs, convs)

	got, err := s.DeleteMessage(context.Background(), moderator, convID, msgID)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("message not tombstoned")
	}
	if len(pub.deleted) != 1 {
		t.Fatalf("PublishDeleted called %d times, want 1", len(pub.deleted))
	}
}

func TestDeleteMessage_PlainMemberCannotDeleteAnothers(t *testing.T) {
	author := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())
	convs := &fakeConvRepo{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindChannelGroup},
		member: &domainconv.Member{UserID: other, Role: domainconv.RoleMember, AdminTier: domainconv.TierNone},
	}
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{
		msgID: {ID: msgID, ConversationID: convID, SenderID: &author, Type: domainmsg.TypeText, Content: "x"},
	}}
	s, _ := newSvc(msgs, convs)
	if _, err := s.DeleteMessage(context.Background(), other, convID, msgID); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
}

func TestDeleteMessage_Idempotent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	m.MarkDeleted(time.Now().UTC())
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	got, err := s.DeleteMessage(context.Background(), actor, convID, m.ID)
	if err != nil {
		t.Fatalf("second DeleteMessage: %v", err)
	}
	if !got.IsDeleted() {
		t.Error("still expected a tombstone")
	}
}

// --- ListMessages --------------------------------------------------------

func TestListMessages_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	_, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, nil, nil, 0)
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListMessages_ConflictingCursors(t *testing.T) {
	convs := groupChatWithMember()
	s, _ := newSvc(&fakeMsgRepo{}, convs)
	a, b := int64(1), int64(2)
	_, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, &a, &b, 0)
	if !errors.Is(err, ErrConflictingCursors) {
		t.Fatalf("err = %v, want ErrConflictingCursors", err)
	}
}

func TestListMessages_ForwardUsesListByConversation(t *testing.T) {
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	s, _ := newSvc(msgs, convs)
	after := int64(5)
	if _, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, &after, nil, 10); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if !msgs.listByConvCalled || msgs.listBeforeCalled {
		t.Fatalf("wrong repo method: byConv=%v before=%v", msgs.listByConvCalled, msgs.listBeforeCalled)
	}
	if msgs.gotAfterSeq != 5 || msgs.gotLimit != 10 {
		t.Errorf("got afterSeq=%d limit=%d", msgs.gotAfterSeq, msgs.gotLimit)
	}
}

func TestListMessages_BackwardUsesListBeforeSeq(t *testing.T) {
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	s, _ := newSvc(msgs, convs)
	before := int64(9)
	if _, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, nil, &before, 0); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if !msgs.listBeforeCalled || msgs.listByConvCalled {
		t.Fatalf("wrong repo method: byConv=%v before=%v", msgs.listByConvCalled, msgs.listBeforeCalled)
	}
	if msgs.gotBeforeSeq != 9 {
		t.Errorf("got beforeSeq=%d, want 9", msgs.gotBeforeSeq)
	}
}

func TestListMessages_NoCursorIsNewestPage(t *testing.T) {
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	s, _ := newSvc(msgs, convs)
	if _, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, nil, nil, 0); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if !msgs.listBeforeCalled || msgs.gotBeforeSeq != 0 {
		t.Errorf("want ListBeforeSeq(beforeSeq=0), got called=%v seq=%d", msgs.listBeforeCalled, msgs.gotBeforeSeq)
	}
}

func TestListMessages_ClampsLimit(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, DefaultPageLimit}, {-3, DefaultPageLimit}, {1000, MaxPageLimit}, {75, 75}} {
		convs := groupChatWithMember()
		msgs := &fakeMsgRepo{}
		s, _ := newSvc(msgs, convs)
		if _, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, nil, nil, tc.in); err != nil {
			t.Fatalf("ListMessages(%d): %v", tc.in, err)
		}
		if msgs.gotLimit != tc.want {
			t.Errorf("limit %d -> repo got %d, want %d", tc.in, msgs.gotLimit, tc.want)
		}
	}
}

func TestPageLimit(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{-1, DefaultPageLimit}, {0, DefaultPageLimit}, {1, 1}, {MaxPageLimit, MaxPageLimit}, {MaxPageLimit + 1, MaxPageLimit}} {
		if got := PageLimit(tc.in); got != tc.want {
			t.Errorf("PageLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// --- publish behaviour -----------------------------------------------------

func TestSendMessage_PublishesCreated(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	s, pub := newSvc(msgs, convs)

	m, err := s.SendMessage(context.Background(), actor, convs.conv.ID, "hi", nil, "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0] != m {
		t.Fatalf("PublishCreated calls = %d, want 1 with the sent message", len(pub.created))
	}
}

func TestSendMessage_IdempotentReplayDoesNotPublish(t *testing.T) {
	convs := groupChatWithMember()
	existing := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Seq: 7}
	msgs := &fakeMsgRepo{
		createErr: domainmsg.ErrDuplicateIdempotencyKey,
		byKey:     map[string]*domainmsg.Message{"k1": existing},
	}
	pub := &fakePublisher{}
	notifProd := &fakeNotifProducer{}
	s := NewService(msgs, convs, pub, notifProd)

	if _, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, "k1"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(pub.created) != 0 {
		t.Fatalf("replay published %d created events, want 0", len(pub.created))
	}
	if notifProd.messageSent != 0 {
		t.Fatalf("replay emitted %d notif intents, want 0", notifProd.messageSent)
	}
}

func TestSendMessage_PublishErrorIsSwallowed(t *testing.T) {
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	pub := &fakePublisher{err: errors.New("nats down")}
	s := NewService(msgs, convs, pub, &fakeNotifProducer{})

	if _, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, ""); err != nil {
		t.Fatalf("SendMessage must succeed despite a publish error: %v", err)
	}
}

func TestEditMessage_PublishesUpdated(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, pub := newSvc(msgs, convs)

	if _, err := s.EditMessage(context.Background(), actor, convs.conv.ID, m.ID, "edited"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if len(pub.updated) != 1 || len(pub.created) != 0 || len(pub.deleted) != 0 {
		t.Fatalf("publish calls: created=%d updated=%d deleted=%d", len(pub.created), len(pub.updated), len(pub.deleted))
	}
}

// --- PostSystemEvent -------------------------------------------------------

func TestPostSystemEvent_KindGuard(t *testing.T) {
	msgs := &fakeMsgRepo{}
	s, pub := newSvc(msgs, &fakeConvRepo{})
	ctx := context.Background()
	actor := uuid.Must(uuid.NewV7())

	// member_* in a broadcast channel -> no-op
	bc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelBroadcast}
	if err := s.PostSystemEvent(ctx, bc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("broadcast member_joined: %v", err)
	}
	if len(pub.created) != 0 {
		t.Fatal("member_joined must not post in a broadcast channel")
	}

	// member_* in a group chat -> posts
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	if err := s.PostSystemEvent(ctx, gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("group member_joined: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0].SystemEvent != domainmsg.SystemMemberJoined {
		t.Fatalf("created = %+v", pub.created)
	}

	// conversation_archived in a broadcast channel -> posts
	if err := s.PostSystemEvent(ctx, bc, &actor, domainmsg.SystemConversationArchived, uuid.Nil, ""); err != nil {
		t.Fatalf("broadcast archived: %v", err)
	}
	if len(pub.created) != 2 {
		t.Fatalf("conversation_archived should post in a broadcast channel")
	}

	// direct -> no-op
	dc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	if err := s.PostSystemEvent(ctx, dc, &actor, domainmsg.SystemConversationArchived, uuid.Nil, ""); err != nil {
		t.Fatalf("direct: %v", err)
	}
	if len(pub.created) != 2 {
		t.Fatal("nothing posts in a direct conversation")
	}

	// member_* in a channel_group -> posts (only channel_broadcast suppresses it)
	cg := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelGroup}
	if err := s.PostSystemEvent(ctx, cg, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("channel_group member_joined: %v", err)
	}
	if len(pub.created) != 3 || pub.created[2].SystemEvent != domainmsg.SystemMemberJoined {
		t.Fatalf("channel_group member_joined must post; created = %+v", pub.created)
	}
}

func TestPostSystemEvent_NotifiesMembershipEvent(t *testing.T) {
	msgs := &fakeMsgRepo{}
	notifProd := &fakeNotifProducer{}
	s := NewService(msgs, &fakeConvRepo{}, &fakePublisher{}, notifProd)
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())

	// a kick emits one membership-event intent
	if err := s.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberRemoved, target, ""); err != nil {
		t.Fatalf("kick: %v", err)
	}
	// a plain join does not
	if err := s.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, target, ""); err != nil {
		t.Fatalf("join: %v", err)
	}
	if notifProd.membershipEvent != 1 {
		t.Fatalf("membershipEvent = %d, want 1 (kick only)", notifProd.membershipEvent)
	}
}

func TestPostSystemEvent_CallEventInDirect(t *testing.T) {
	s, pub := newSvc(&fakeMsgRepo{}, &fakeConvRepo{})
	direct := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	if err := s.PostSystemEvent(context.Background(), direct, nil, domainmsg.SystemCallStarted, uuid.Must(uuid.NewV7()), ""); err != nil {
		t.Fatalf("PostSystemEvent: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0].SystemEvent != domainmsg.SystemCallStarted {
		t.Fatalf("expected one call_started system message, got %+v", pub.created)
	}
}

func TestPostSystemEvent_PersistErrorReturned(t *testing.T) {
	msgs := &fakeMsgRepo{createErr: errors.New("db down")}
	s, _ := newSvc(msgs, &fakeConvRepo{})
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	if err := s.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err == nil {
		t.Fatal("want a non-nil error when messages.Create fails")
	}
}

func TestPostSystemEvent_PublishErrorSwallowed(t *testing.T) {
	msgs := &fakeMsgRepo{}
	pub := &fakePublisher{err: errors.New("nats down")}
	s := NewService(msgs, &fakeConvRepo{}, pub, &fakeNotifProducer{})
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	if err := s.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("a publish error must be swallowed, got %v", err)
	}
	if len(pub.created) != 1 {
		t.Fatalf("the event should still have been persisted + publish attempted: %+v", pub.created)
	}
}

func TestDeleteMessage_PublishesDeleted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, pub := newSvc(msgs, convs)

	if _, err := s.DeleteMessage(context.Background(), actor, convs.conv.ID, m.ID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if len(pub.deleted) != 1 || len(pub.created) != 0 || len(pub.updated) != 0 {
		t.Fatalf("publish calls: created=%d updated=%d deleted=%d", len(pub.created), len(pub.updated), len(pub.deleted))
	}
}

func TestMarkRead_StoresCursorOnlyForMessageInConversation(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s, _ := newSvc(msgs, convs)

	if err := s.MarkRead(context.Background(), actor, convs.conv.ID, m.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if got := convs.readCursor; got != [3]uuid.UUID{convs.conv.ID, actor, m.ID} {
		t.Fatalf("read cursor = %v, want conversation, actor, message", got)
	}

	m.ConversationID = uuid.Must(uuid.NewV7())
	if err := s.MarkRead(context.Background(), actor, convs.conv.ID, m.ID); !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("cross-conversation cursor error = %v, want not found", err)
	}
}
