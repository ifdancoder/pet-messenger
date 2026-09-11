package call_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/call"
)

func TestNew(t *testing.T) {
	conv := uuid.Must(uuid.NewV7())
	initiator := uuid.Must(uuid.NewV7())

	c, err := call.New(conv, initiator, "node-a")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.ConversationID != conv || c.InitiatorID != initiator || c.MediaNodeID != "node-a" {
		t.Fatalf("c = %+v", c)
	}
	if c.ID.Version() != 7 || c.Status != call.StatusActive || c.StartedAt.IsZero() || c.EndedAt != nil {
		t.Fatalf("id/status/timestamps: %+v", c)
	}
	if !c.IsLive() {
		t.Fatal("fresh call should be live")
	}

	for name, tc := range map[string]struct {
		conv, initiator uuid.UUID
		node            string
		want            error
	}{
		"nil conv":      {uuid.Nil, initiator, "n", call.ErrInvalidConversation},
		"nil initiator": {conv, uuid.Nil, "n", call.ErrInvalidInitiator},
		"empty node":    {conv, initiator, "", call.ErrInvalidNode},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := call.New(tc.conv, tc.initiator, tc.node); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCall_End(t *testing.T) {
	c, _ := call.New(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "n")
	if err := c.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if c.Status != call.StatusEnded || c.EndedAt == nil || c.IsLive() {
		t.Fatalf("after End: %+v", c)
	}
	if err := c.End(); !errors.Is(err, call.ErrAlreadyEnded) {
		t.Fatalf("second End: %v, want ErrAlreadyEnded", err)
	}
}

func TestNewParticipant(t *testing.T) {
	callID := uuid.Must(uuid.NewV7())
	uid := uuid.Must(uuid.NewV7())

	p, err := call.NewParticipant(callID, uid, call.RoleViewer)
	if err != nil {
		t.Fatalf("NewParticipant: %v", err)
	}
	if p.CallID != callID || p.UserID != uid || p.Role != call.RoleViewer {
		t.Fatalf("p = %+v", p)
	}
	if p.ID.Version() != 7 || p.JoinedAt.IsZero() || p.LeftAt != nil {
		t.Fatalf("id/timestamps: %+v", p)
	}
	if _, err := call.NewParticipant(callID, uid, call.Role("spectator")); !errors.Is(err, call.ErrInvalidRole) {
		t.Fatalf("bad role: %v", err)
	}
	if _, err := call.NewParticipant(uuid.Nil, uid, call.RolePublisher); !errors.Is(err, call.ErrInvalidCall) {
		t.Fatalf("nil call: %v", err)
	}
}

func TestRoleStatusValid(t *testing.T) {
	if !call.RolePublisher.Valid() || !call.RoleViewer.Valid() || call.Role("x").Valid() {
		t.Fatal("Role.Valid")
	}
	if !call.StatusActive.Valid() || !call.StatusEnded.Valid() || call.Status("x").Valid() {
		t.Fatal("Status.Valid")
	}
}
