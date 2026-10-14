// internal/message/application/command/send_message_test.go
package command

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

func TestSendMessage_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	loader := &fakeMessageLoader{}
	writer := &fakeMessageWriter{}
	notifProd := &fakeNotifProducer{}
	h := NewSendMessageHandler(loader, writer, convs, &fakePublisher{}, notifProd)

	m, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: actor, ConversationID: convID, Content: "  hi  "})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(writer.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(writer.created))
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
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi"})
	if !errors.Is(err, domainconv.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestSendMessage_ConversationNotFound(t *testing.T) {
	convs := &fakeConversationMembership{convErr: domainconv.ErrNotFound}
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7()), Content: "hi"})
	if !errors.Is(err, domainconv.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSendMessage_CannotPost(t *testing.T) {
	convs := &fakeConversationMembership{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelBroadcast},
		member: &domainconv.Member{Role: domainconv.RoleSubscriber},
	}
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi"})
	if !errors.Is(err, ErrCannotPost) {
		t.Fatalf("err = %v, want ErrCannotPost", err)
	}
}

func TestSendMessage_ReplyTargetMissing(t *testing.T) {
	convs := groupChatWithMember()
	h := NewSendMessageHandler(&fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{}}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	missing := uuid.Must(uuid.NewV7())
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi", ReplyToID: &missing})
	if !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("err = %v, want ErrReplyNotFound", err)
	}
}

func TestSendMessage_ReplyTargetOtherConversation(t *testing.T) {
	convs := groupChatWithMember()
	other := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7())}
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{other.ID: other}}
	h := NewSendMessageHandler(loader, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi", ReplyToID: &other.ID})
	if !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("err = %v, want ErrReplyNotFound", err)
	}
}

func TestSendMessage_IdempotentReplay(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	existing := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: 7, Content: "hi"}
	loader := &fakeMessageLoader{byKey: map[string]*domainmsg.Message{"k1": existing}}
	writer := &fakeMessageWriter{createErr: domainmsg.ErrDuplicateIdempotencyKey}
	h := NewSendMessageHandler(loader, writer, convs, &fakePublisher{}, &fakeNotifProducer{})

	m, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convID, Content: "hi", IdempotencyKey: "k1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if m.ID != existing.ID || m.Seq != 7 {
		t.Errorf("got id=%v seq=%d, want the existing id=%v seq=7", m.ID, m.Seq, existing.ID)
	}
}

func TestSendMessage_ArchivedRejected(t *testing.T) {
	now := time.Now()
	convs := &fakeConversationMembership{
		conv:   &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat, ArchivedAt: &now},
		member: &domainconv.Member{Role: domainconv.RoleMember},
	}
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	if _, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi"}); !errors.Is(err, ErrConversationArchived) {
		t.Fatalf("err = %v, want ErrConversationArchived", err)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	convs := groupChatWithMember()
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{}, &fakeNotifProducer{})
	_, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "   "})
	if !errors.Is(err, domainmsg.ErrEmptyContent) {
		t.Fatalf("err = %v, want ErrEmptyContent", err)
	}
}

func TestSendMessage_PublishesCreated(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	pub := &fakePublisher{}
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, pub, &fakeNotifProducer{})

	m, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: actor, ConversationID: convs.conv.ID, Content: "hi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0] != m {
		t.Fatalf("PublishCreated calls = %d, want 1 with the sent message", len(pub.created))
	}
}

func TestSendMessage_IdempotentReplayDoesNotPublish(t *testing.T) {
	convs := groupChatWithMember()
	existing := &domainmsg.Message{ID: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Seq: 7}
	loader := &fakeMessageLoader{byKey: map[string]*domainmsg.Message{"k1": existing}}
	writer := &fakeMessageWriter{createErr: domainmsg.ErrDuplicateIdempotencyKey}
	pub := &fakePublisher{}
	notifProd := &fakeNotifProducer{}
	h := NewSendMessageHandler(loader, writer, convs, pub, notifProd)

	if _, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi", IdempotencyKey: "k1"}); err != nil {
		t.Fatalf("Handle: %v", err)
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
	pub := &fakePublisher{err: errors.New("nats down")}
	h := NewSendMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, pub, &fakeNotifProducer{})

	if _, err := h.Handle(context.Background(), in.SendMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, Content: "hi"}); err != nil {
		t.Fatalf("Handle must succeed despite a publish error: %v", err)
	}
}
