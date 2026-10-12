package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/conversation/domain"
)

func TestEventTypes(t *testing.T) {
	cases := []struct {
		name string
		ev   domain.Event
		want string
	}{
		{"created", domain.ConversationCreated{}, "conversation.created"},
		{"member added", domain.MemberAdded{}, "member.added"},
		{"member removed", domain.MemberRemoved{}, "member.removed"},
		{"tier changed", domain.MemberTierChanged{}, "member.tier_changed"},
		{"archived", domain.ConversationArchived{}, "conversation.archived"},
		{"unarchived", domain.ConversationUnarchived{}, "conversation.unarchived"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ev.EventType(); got != tc.want {
				t.Fatalf("EventType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMemberAddedCarriesDirectPeer(t *testing.T) {
	peer := uuid.Must(uuid.NewV7())
	ev := domain.MemberAdded{ConversationID: uuid.Must(uuid.NewV7()), UserID: uuid.Must(uuid.NewV7()), Kind: domain.KindDirect, DirectPeerUserID: &peer}
	if ev.DirectPeerUserID == nil || *ev.DirectPeerUserID != peer {
		t.Fatalf("DirectPeerUserID = %v, want %v", ev.DirectPeerUserID, peer)
	}
}
