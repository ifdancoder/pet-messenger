package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/query"
	"pet-messenger/internal/conversation/domain"
)

func TestListMembers_ActorMustBeMember(t *testing.T) {
	convID, actor := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := &fakeLoaderForQuery{conv: &domain.Conversation{ID: convID}, members: map[uuid.UUID]*domain.Member{}}
	view := &fakeMemberView{profiles: []*domain.MemberProfile{{Member: domain.Member{UserID: actor}}}}
	h := query.NewListMembersHandler(loader, view)

	_, err := h.Handle(context.Background(), in.ListMembersQuery{Actor: actor, ConversationID: convID})
	if err != domain.ErrNotMember {
		t.Fatalf("err = %v, want ErrNotMember (actor is not yet a member)", err)
	}
}

func TestListMembers_ReturnsRoster(t *testing.T) {
	convID, actor := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := &fakeLoaderForQuery{conv: &domain.Conversation{ID: convID}, members: map[uuid.UUID]*domain.Member{actor: {UserID: actor}}}
	view := &fakeMemberView{profiles: []*domain.MemberProfile{{Member: domain.Member{UserID: actor}}}}
	h := query.NewListMembersHandler(loader, view)

	got, err := h.Handle(context.Background(), in.ListMembersQuery{Actor: actor, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d members, want 1", len(got))
	}
}
