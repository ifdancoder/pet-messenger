package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/query"
	"pet-messenger/internal/conversation/domain"
)

func TestListPublicChannels_TrimsQueryAndClampsLimit(t *testing.T) {
	view := &fakePublicChannelView{channels: []*domain.Conversation{{ID: uuid.Must(uuid.NewV7())}}}
	h := query.NewListPublicChannelsHandler(view)

	got, err := h.Handle(context.Background(), in.ListPublicChannelsQuery{Query: "  news  ", Limit: 0})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d channels, want 1", len(got))
	}
	if view.lastArgs.query != "news" {
		t.Fatalf("query passed to view = %q, want %q", view.lastArgs.query, "news")
	}
	if view.lastArgs.limit != 20 { // DefaultListLimit
		t.Fatalf("limit passed to view = %d, want 20", view.lastArgs.limit)
	}
}
