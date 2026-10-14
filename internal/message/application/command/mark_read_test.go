// internal/message/application/command/mark_read_test.go
package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/message/application/port/in"
	domainmsg "pet-messenger/internal/message/domain"
)

func TestMarkRead_StoresCursorOnlyForMessageInConversation(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convs := groupChatWithMember()
	m := ownText(t, actor, convs.conv.ID)
	loader := &fakeMessageLoader{byID: map[uuid.UUID]*domainmsg.Message{m.ID: m}}
	cursor := &fakeReadCursor{}
	h := NewMarkReadHandler(loader, convs, cursor)

	if err := h.Handle(context.Background(), in.MarkReadCommand{Actor: actor, ConversationID: convs.conv.ID, MessageID: m.ID}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := cursor.readCursor; got != [3]uuid.UUID{convs.conv.ID, actor, m.ID} {
		t.Fatalf("read cursor = %v, want conversation, actor, message", got)
	}

	m.ConversationID = uuid.Must(uuid.NewV7())
	if err := h.Handle(context.Background(), in.MarkReadCommand{Actor: actor, ConversationID: convs.conv.ID, MessageID: m.ID}); !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("cross-conversation cursor error = %v, want not found", err)
	}
}
