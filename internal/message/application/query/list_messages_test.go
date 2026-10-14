// internal/message/application/query/list_messages_test.go
package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/message/application/port/in"
	domainmsg "pet-messenger/internal/message/domain"
)

type fakeLoader struct {
	byID map[uuid.UUID]*domainmsg.Message
}

func (f *fakeLoader) GetByID(_ context.Context, id uuid.UUID) (*domainmsg.Message, error) {
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return nil, domainmsg.ErrNotFound
}
func (f *fakeLoader) GetByIdempotencyKey(context.Context, uuid.UUID, uuid.UUID, string) (*domainmsg.Message, error) {
	return nil, domainmsg.ErrNotFound
}

type fakeHistory struct {
	listByConvCalled bool
	listBeforeCalled bool
	gotAfterSeq      int64
	gotBeforeSeq     int64
	gotLimit         int
	result           []*domainmsg.Message
}

func (f *fakeHistory) ListByConversation(_ context.Context, _ uuid.UUID, afterSeq int64, limit int) ([]*domainmsg.Message, error) {
	f.listByConvCalled, f.gotAfterSeq, f.gotLimit = true, afterSeq, limit
	return f.result, nil
}
func (f *fakeHistory) ListBeforeSeq(_ context.Context, _ uuid.UUID, beforeSeq int64, limit int) ([]*domainmsg.Message, error) {
	f.listBeforeCalled, f.gotBeforeSeq, f.gotLimit = true, beforeSeq, limit
	return f.result, nil
}

type fakeMembership struct {
	member    *domainconv.Member
	memberErr error
}

func (f *fakeMembership) GetByID(context.Context, uuid.UUID) (*domainconv.Conversation, error) {
	return nil, nil
}
func (f *fakeMembership) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return f.member, f.memberErr
}

type fakeRoster struct{ members []*domainconv.Member }

func (f *fakeRoster) ListMembers(context.Context, uuid.UUID, bool, time.Time, uuid.UUID, int) ([]*domainconv.Member, error) {
	return f.members, nil
}

func newHandler() (in.ListMessagesUseCase, *fakeHistory, *fakeMembership) {
	hist := &fakeHistory{}
	memb := &fakeMembership{member: &domainconv.Member{Role: domainconv.RoleMember}}
	return NewListMessagesHandler(&fakeLoader{}, hist, memb, &fakeRoster{}), hist, memb
}

func TestListMessages_NotMember(t *testing.T) {
	h, _, memb := newHandler()
	memb.member, memb.memberErr = nil, domainconv.ErrNotMember
	convID := uuid.Must(uuid.NewV7())
	_, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: convID})
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestListMessages_ConflictingCursors(t *testing.T) {
	h, _, _ := newHandler()
	a, b := int64(1), int64(2)
	_, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), AfterSeq: &a, BeforeSeq: &b})
	if !errors.Is(err, ErrConflictingCursors) {
		t.Fatalf("err = %v, want ErrConflictingCursors", err)
	}
}

func TestListMessages_ForwardUsesListByConversation(t *testing.T) {
	h, hist, _ := newHandler()
	after := int64(5)
	if _, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), AfterSeq: &after, Limit: 10}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !hist.listByConvCalled || hist.listBeforeCalled {
		t.Fatalf("wrong history method: byConv=%v before=%v", hist.listByConvCalled, hist.listBeforeCalled)
	}
	if hist.gotAfterSeq != 5 || hist.gotLimit != 10 {
		t.Errorf("got afterSeq=%d limit=%d", hist.gotAfterSeq, hist.gotLimit)
	}
}

func TestListMessages_BackwardUsesListBeforeSeq(t *testing.T) {
	h, hist, _ := newHandler()
	before := int64(9)
	if _, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), BeforeSeq: &before}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !hist.listBeforeCalled || hist.listByConvCalled {
		t.Fatalf("wrong history method: byConv=%v before=%v", hist.listByConvCalled, hist.listBeforeCalled)
	}
	if hist.gotBeforeSeq != 9 {
		t.Errorf("got beforeSeq=%d, want 9", hist.gotBeforeSeq)
	}
}

func TestListMessages_NoCursorIsNewestPage(t *testing.T) {
	h, hist, _ := newHandler()
	if _, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7())}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !hist.listBeforeCalled || hist.gotBeforeSeq != 0 {
		t.Errorf("want ListBeforeSeq(beforeSeq=0), got called=%v seq=%d", hist.listBeforeCalled, hist.gotBeforeSeq)
	}
}

func TestListMessages_ClampsLimit(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, DefaultPageLimit}, {-3, DefaultPageLimit}, {1000, MaxPageLimit}, {75, 75}} {
		h, hist, _ := newHandler()
		if _, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), Limit: tc.in}); err != nil {
			t.Fatalf("Handle(%d): %v", tc.in, err)
		}
		if hist.gotLimit != tc.want {
			t.Errorf("limit %d -> history got %d, want %d", tc.in, hist.gotLimit, tc.want)
		}
	}
}

// TestListMessages_AnnotatesReadStatusFromRosterAndLoader exercises the
// dual-port read-status path: a non-empty roster (out.ConversationRosterPort)
// supplies each member's LastReadMessageID, out.MessageLoader resolves those
// IDs to real messages (for their Seq), and a non-empty history page
// (out.MessageHistoryPort) is annotated against both. Without the loader
// actually being called, ReadBy/RecipientCount could not be computed at all.
func TestListMessages_AnnotatesReadStatusFromRosterAndLoader(t *testing.T) {
	convID := uuid.Must(uuid.NewV7())
	alice, bob, carol := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	aliceReadID, bobReadID, carolReadID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	// Each member's last-read message, resolved via the loader port.
	loader := &fakeLoader{byID: map[uuid.UUID]*domainmsg.Message{
		aliceReadID: {ID: aliceReadID, ConversationID: convID, Seq: 20},
		bobReadID:   {ID: bobReadID, ConversationID: convID, Seq: 8},
		carolReadID: {ID: carolReadID, ConversationID: convID, Seq: 12},
	}}

	roster := &fakeRoster{members: []*domainconv.Member{
		{ConversationID: convID, UserID: alice, Role: domainconv.RoleMember, LastReadMessageID: &aliceReadID},
		{ConversationID: convID, UserID: bob, Role: domainconv.RoleMember, LastReadMessageID: &bobReadID},
		{ConversationID: convID, UserID: carol, Role: domainconv.RoleMember, LastReadMessageID: &carolReadID},
	}}

	// Three messages from three different senders, so every member's
	// read cursor gets exercised as both "the sender" (excluded from their
	// own message's recipient count) and "a recipient" of the other two.
	m1 := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: 5, SenderID: &bob}
	m2 := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: 10, SenderID: &alice}
	m3 := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: 15, SenderID: &carol}

	hist := &fakeHistory{result: []*domainmsg.Message{m1, m2, m3}}
	memb := &fakeMembership{member: &domainconv.Member{Role: domainconv.RoleMember}}

	h := NewListMessagesHandler(loader, hist, memb, roster)

	before := int64(100)
	got, err := h.Handle(context.Background(), in.ListMessagesQuery{Actor: alice, ConversationID: convID, BeforeSeq: &before})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}

	// Expected per message: RecipientCount = members minus the sender (2),
	// since all three roster members never leave; ReadBy = how many of
	// those 2 non-sender members have a last-read Seq >= the message's Seq.
	//   m1 (Seq=5,  sender=bob):   alice(20)>=5 yes, carol(12)>=5 yes  -> ReadBy=2
	//   m2 (Seq=10, sender=alice): bob(8)>=10 no,    carol(12)>=10 yes -> ReadBy=1
	//   m3 (Seq=15, sender=carol): alice(20)>=15 yes, bob(8)>=15 no    -> ReadBy=1
	cases := []struct {
		name           string
		msg            *domainmsg.Message
		wantReadBy     int
		wantRecipients int
	}{
		{"m1 sent by bob", m1, 2, 2},
		{"m2 sent by alice", m2, 1, 2},
		{"m3 sent by carol", m3, 1, 2},
	}
	for _, tc := range cases {
		if tc.msg.ReadBy != tc.wantReadBy || tc.msg.RecipientCount != tc.wantRecipients {
			t.Errorf("%s: ReadBy=%d RecipientCount=%d, want ReadBy=%d RecipientCount=%d",
				tc.name, tc.msg.ReadBy, tc.msg.RecipientCount, tc.wantReadBy, tc.wantRecipients)
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
