package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	in "pet-messenger/internal/call/application/port/in"
	conversation "pet-messenger/internal/conversation/domain"
)

func TestUnmuteParticipant_Authorization(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tier            conversation.AdminTier
		initiator, self bool
		want            error
	}{
		{name: "initiator", initiator: true},
		{name: "admin", tier: conversation.TierAdmin},
		{name: "plain member", tier: conversation.TierNone, want: ErrForbidden},
		{name: "self unmute denied", initiator: true, self: true, want: ErrCannotTargetSelf},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := newFakeCallStore()
			convs := newFakeConversationReader()
			rooms := &fakeRoomControl{}
			h := NewUnmuteParticipantHandler(calls, convs, &fakeMediaNodeRegistry{}, rooms)
			convID, _ := groupChat(t, convs)
			c := mustCall(t, convID, uuid.Must(uuid.NewV7()))
			calls.seedLive(c)
			actor := addMember(convs, convID, tc.tier)
			if tc.initiator {
				actor = c.InitiatorID
			}
			target := addMember(convs, convID, conversation.TierNone)
			if tc.self {
				target = actor
			}
			err := h.Handle(context.Background(), in.UnmuteParticipantCommand{Actor: actor, CallID: c.ID, TargetID: target})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Handle = %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				if len(rooms.unmuted) != 0 {
					t.Fatal("unauthorized request reached media node")
				}
				return
			}
			if len(rooms.unmuted) != 1 || rooms.unmuted[0] != c.ID.String()+"/"+target.String() {
				t.Fatalf("unmuted = %v", rooms.unmuted)
			}
		})
	}
}
