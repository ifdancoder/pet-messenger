package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/query"
	"pet-messenger/internal/conversation/domain"
	domainuser "pet-messenger/internal/user/domain"
)

func TestListMembers_ActorMustBeMember(t *testing.T) {
	convID, actor := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := &fakeLoaderForQuery{conv: &domain.Conversation{ID: convID}, members: map[uuid.UUID]*domain.Member{}}
	view := &fakeMemberView{members: []*domain.Member{{UserID: actor}}}
	h := query.NewListMembersHandler(loader, view, &fakeProfileView{})

	_, err := h.Handle(context.Background(), in.ListMembersQuery{Actor: actor, ConversationID: convID})
	if err != domain.ErrNotMember {
		t.Fatalf("err = %v, want ErrNotMember (actor is not yet a member)", err)
	}
}

func TestListMembers_ReturnsRoster(t *testing.T) {
	convID, actor := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := &fakeLoaderForQuery{conv: &domain.Conversation{ID: convID}, members: map[uuid.UUID]*domain.Member{actor: {UserID: actor}}}
	view := &fakeMemberView{members: []*domain.Member{{UserID: actor}}}
	h := query.NewListMembersHandler(loader, view, &fakeProfileView{})

	got, err := h.Handle(context.Background(), in.ListMembersQuery{Actor: actor, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d members, want 1", len(got))
	}
}

// TestListMembers_EnrichesEachMemberByID exercises the zip in Handle: it
// must index GetProfilesByIDs' result by Profile.ID, not by position. The
// fake profile view below deliberately returns profiles in the REVERSE order
// of the members slice — if Handle zipped by position instead of by ID,
// every member would come back with the wrong Username/DisplayName/AvatarURL.
func TestListMembers_EnrichesEachMemberByID(t *testing.T) {
	convID, actor := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	userA, userB := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	loader := &fakeLoaderForQuery{
		conv:    &domain.Conversation{ID: convID},
		members: map[uuid.UUID]*domain.Member{actor: {UserID: actor}},
	}
	view := &fakeMemberView{members: []*domain.Member{
		{UserID: userA},
		{UserID: userB},
	}}
	profiles := &fakeProfileView{profiles: []*domainuser.Profile{
		// Deliberately reversed relative to view.members above.
		{ID: userB, Username: "bee", DisplayName: "Bee", AvatarURL: "bee.png"},
		{ID: userA, Username: "ay", DisplayName: "Ay", AvatarURL: "ay.png"},
	}}
	h := query.NewListMembersHandler(loader, view, profiles)

	got, err := h.Handle(context.Background(), in.ListMembersQuery{Actor: actor, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d members, want 2", len(got))
	}
	byUser := make(map[uuid.UUID]*domain.MemberProfile, len(got))
	for _, mp := range got {
		byUser[mp.UserID] = mp
	}
	a, ok := byUser[userA]
	if !ok {
		t.Fatalf("missing member %s in result", userA)
	}
	if a.Username != "ay" || a.DisplayName != "Ay" || a.AvatarURL != "ay.png" {
		t.Fatalf("userA profile = %+v, want {ay Ay ay.png}", a)
	}
	b, ok := byUser[userB]
	if !ok {
		t.Fatalf("missing member %s in result", userB)
	}
	if b.Username != "bee" || b.DisplayName != "Bee" || b.AvatarURL != "bee.png" {
		t.Fatalf("userB profile = %+v, want {bee Bee bee.png}", b)
	}
}
