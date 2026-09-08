package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
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
	conv      *domainconv.Conversation
	convErr   error
	member    *domainconv.Member
	memberErr error
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
	s := NewService(msgs, convs)

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
}

func TestSendMessage_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	s := NewService(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, "")
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestSendMessage_ConversationNotFound(t *testing.T) {
	convs := &fakeConvRepo{convErr: domainconv.ErrNotFound}
	s := NewService(&fakeMsgRepo{}, convs)
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
	s := NewService(&fakeMsgRepo{}, convs)
	_, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, "hi", nil, "")
	if !errors.Is(err, ErrCannotPost) {
		t.Fatalf("err = %v, want ErrCannotPost", err)
	}
}

func TestSendMessage_ReplyTargetMissing(t *testing.T) {
	convs := groupChatWithMember()
	s := NewService(&fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{}}, convs)
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
	s := NewService(msgs, convs)
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
	s := NewService(msgs, convs)

	m, err := s.SendMessage(context.Background(), uuid.Must(uuid.NewV7()), convID, "hi", nil, "k1")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if m.ID != existing.ID || m.Seq != 7 {
		t.Errorf("got id=%v seq=%d, want the existing id=%v seq=7", m.ID, m.Seq, existing.ID)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	convs := groupChatWithMember()
	s := NewService(&fakeMsgRepo{}, convs)
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
	s := NewService(msgs, convs)

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
	s := NewService(msgs, convs)

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
	s := NewService(msgs, convs)

	_, err := s.EditMessage(context.Background(), actor, convs.conv.ID, m.ID, "x")
	if !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEditMessage_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	s := NewService(&fakeMsgRepo{}, convs)
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
	s := NewService(msgs, convs)

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
	s := NewService(msgs, convs)

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
	s := NewService(msgs, convs)

	_, err := s.DeleteMessage(context.Background(), uuid.Must(uuid.NewV7()), convID, m.ID)
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
	if len(msgs.updated) != 0 {
		t.Errorf("Update called %d times, want 0", len(msgs.updated))
	}
}

func TestDeleteMessage_Idempotent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	m.MarkDeleted(time.Now().UTC())
	msgs := &fakeMsgRepo{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	s := NewService(msgs, convs)

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
	s := NewService(&fakeMsgRepo{}, convs)
	_, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, nil, nil, 0)
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListMessages_ConflictingCursors(t *testing.T) {
	convs := groupChatWithMember()
	s := NewService(&fakeMsgRepo{}, convs)
	a, b := int64(1), int64(2)
	_, err := s.ListMessages(context.Background(), uuid.Must(uuid.NewV7()), convs.conv.ID, &a, &b, 0)
	if !errors.Is(err, ErrConflictingCursors) {
		t.Fatalf("err = %v, want ErrConflictingCursors", err)
	}
}

func TestListMessages_ForwardUsesListByConversation(t *testing.T) {
	convs := groupChatWithMember()
	msgs := &fakeMsgRepo{}
	s := NewService(msgs, convs)
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
	s := NewService(msgs, convs)
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
	s := NewService(msgs, convs)
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
		s := NewService(msgs, convs)
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
