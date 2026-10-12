package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/application/query"
)

func TestListMine_ReturnsInboxEntries(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	view := &fakeInboxView{entries: []*out.InboxEntry{{ConversationID: uuid.Must(uuid.NewV7())}}}
	h := query.NewListMineHandler(view)

	got, err := h.Handle(context.Background(), in.ListMineQuery{UserID: me, Limit: 0})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
}
