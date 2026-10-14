// internal/message/application/command/edit_message_test.go
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

// ownText builds a text message authored by actor in convID, for tests that
// need an existing message to edit or delete. Shared by edit_message_test.go
// and delete_message_test.go.
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
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	writer := &fakeMessageWriter{}
	h := NewEditMessageHandler(loader, writer, convs, &fakePublisher{})

	got, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: actor, ConversationID: convID, MessageID: m.ID, Content: "  edited  "})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.Content != "edited" || !got.IsEdited() {
		t.Errorf("got %+v", got)
	}
	if len(writer.updated) != 1 {
		t.Fatalf("Update called %d times, want 1", len(writer.updated))
	}
}

func TestEditMessage_NotAuthor(t *testing.T) {
	convs := groupChatWithMember()
	convID := convs.conv.ID
	m := ownText(t, uuid.Must(uuid.NewV7()), convID) // authored by someone else
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	writer := &fakeMessageWriter{}
	h := NewEditMessageHandler(loader, writer, convs, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convID, MessageID: m.ID, Content: "x"})
	if !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("err = %v, want ErrNotAuthor", err)
	}
	if len(writer.updated) != 0 {
		t.Errorf("Update called %d times, want 0", len(writer.updated))
	}
}

func TestEditMessage_WrongConversation(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, uuid.Must(uuid.NewV7())) // belongs to another conversation
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	h := NewEditMessageHandler(loader, &fakeMessageWriter{}, convs, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: actor, ConversationID: convs.conv.ID, MessageID: m.ID, Content: "x"})
	if !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEditMessage_NotMember(t *testing.T) {
	convs := groupChatWithMember()
	convs.member, convs.memberErr = nil, domainconv.ErrNotMember
	h := NewEditMessageHandler(&fakeMessageLoader{}, &fakeMessageWriter{}, convs, &fakePublisher{})
	_, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: uuid.Must(uuid.NewV7()), ConversationID: convs.conv.ID, MessageID: uuid.Must(uuid.NewV7()), Content: "x"})
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
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	h := NewEditMessageHandler(loader, &fakeMessageWriter{}, convs, &fakePublisher{})

	_, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: actor, ConversationID: convID, MessageID: m.ID, Content: "x"})
	if !errors.Is(err, domainmsg.ErrNotEditable) {
		t.Fatalf("err = %v, want ErrNotEditable", err)
	}
}

func TestEditMessage_PublishesUpdated(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	pub := &fakePublisher{}
	h := NewEditMessageHandler(loader, &fakeMessageWriter{}, convs, pub)

	if _, err := h.Handle(context.Background(), in.EditMessageCommand{Actor: actor, ConversationID: convs.conv.ID, MessageID: m.ID, Content: "edited"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(pub.updated) != 1 || len(pub.created) != 0 || len(pub.deleted) != 0 {
		t.Fatalf("publish calls: created=%d updated=%d deleted=%d", len(pub.created), len(pub.updated), len(pub.deleted))
	}
}
