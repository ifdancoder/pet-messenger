// internal/message/application/command/delete_message_test.go
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

func TestDeleteMessage_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	writer := &fakeMessageWriter{}
	h := NewDeleteMessageHandler(loader, writer, convs, &fakePublisher{})

	got, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: actor, ConversationID: convID, MessageID: m.ID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !got.IsDeleted() || got.Content != "" {
		t.Errorf("got %+v, want tombstoned", got)
	}
	if len(writer.updated) != 1 {
		t.Fatalf("Update called %d times, want 1", len(writer.updated))
	}
}

func TestDeleteMessage_NotAuthor(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, uuid.Must(uuid.NewV7()), convID)
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	writer := &fakeMessageWriter{}
	h := NewDeleteMessageHandler(loader, writer, convs, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convID, MessageID: m.ID})
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
	if len(writer.updated) != 0 {
		t.Errorf("Update called %d times, want 0", len(writer.updated))
	}
}

func TestDeleteMessage_ContentAdminDeletesAnothersMessage(t *testing.T) {
	author := uuid.Must(uuid.NewV7())
	moderator := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())

	convs := &fakeConversationMembership{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindChannelGroup},
		member: &domainconv.Member{UserID: moderator, Role: domainconv.RoleMember, AdminTier: domainconv.TierContentAdmin},
	}
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{
		msgID: {ID: msgID, ConversationID: convID, SenderID: &author, Type: domainmsg.TypeText, Content: "x"},
	}}
	pub := &fakePublisher{}
	h := NewDeleteMessageHandler(loader, &fakeMessageWriter{}, convs, pub)

	got, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: moderator, ConversationID: convID, MessageID: msgID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
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
	convs := &fakeConversationMembership{
		conv:   &domainconv.Conversation{ID: convID, Kind: domainconv.KindChannelGroup},
		member: &domainconv.Member{UserID: other, Role: domainconv.RoleMember, AdminTier: domainconv.TierNone},
	}
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{
		msgID: {ID: msgID, ConversationID: convID, SenderID: &author, Type: domainmsg.TypeText, Content: "x"},
	}}
	h := NewDeleteMessageHandler(loader, &fakeMessageWriter{}, convs, &fakePublisher{})
	if _, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: other, ConversationID: convID, MessageID: msgID}); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
}

func TestDeleteMessage_Idempotent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, actor, convID)
	m.MarkDeleted(time.Now().UTC())
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	h := NewDeleteMessageHandler(loader, &fakeMessageWriter{}, convs, &fakePublisher{})

	got, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: actor, ConversationID: convID, MessageID: m.ID})
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if !got.IsDeleted() {
		t.Error("still expected a tombstone")
	}
}

func TestDeleteMessage_PublishesDeleted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	pub := &fakePublisher{}
	h := NewDeleteMessageHandler(loader, &fakeMessageWriter{}, convs, pub)

	if _, err := h.Handle(context.Background(), in.DeleteMessageCommand{Actor: actor, ConversationID: convs.conv.ID, MessageID: m.ID}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(pub.deleted) != 1 || len(pub.created) != 0 || len(pub.updated) != 0 {
		t.Fatalf("publish calls: created=%d updated=%d deleted=%d", len(pub.created), len(pub.updated), len(pub.deleted))
	}
}
