package domain_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/call/domain"
)

func TestNew(t *testing.T) {
	conv := uuid.Must(uuid.NewV7())
	initiator := uuid.Must(uuid.NewV7())

	c, err := domain.New(conv, initiator, "node-a")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.ConversationID != conv || c.InitiatorID != initiator || c.MediaNodeID != "node-a" {
		t.Fatalf("c = %+v", c)
	}
	if c.ID.Version() != 7 || c.Status != domain.StatusActive || c.StartedAt.IsZero() || c.EndedAt != nil {
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
		"nil conv":      {uuid.Nil, initiator, "n", domain.ErrInvalidConversation},
		"nil initiator": {conv, uuid.Nil, "n", domain.ErrInvalidInitiator},
		"empty node":    {conv, initiator, "", domain.ErrInvalidNode},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.New(tc.conv, tc.initiator, tc.node); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCall_End(t *testing.T) {
	c, _ := domain.New(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "n")
	if err := c.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if c.Status != domain.StatusEnded || c.EndedAt == nil || c.IsLive() {
		t.Fatalf("after End: %+v", c)
	}
	if err := c.End(); !errors.Is(err, domain.ErrAlreadyEnded) {
		t.Fatalf("second End: %v, want ErrAlreadyEnded", err)
	}
}

func TestNewParticipant(t *testing.T) {
	callID := uuid.Must(uuid.NewV7())
	uid := uuid.Must(uuid.NewV7())

	p, err := domain.NewParticipant(callID, uid, domain.RoleViewer)
	if err != nil {
		t.Fatalf("NewParticipant: %v", err)
	}
	if p.CallID != callID || p.UserID != uid || p.Role != domain.RoleViewer {
		t.Fatalf("p = %+v", p)
	}
	if p.ID.Version() != 7 || p.JoinedAt.IsZero() || p.LeftAt != nil {
		t.Fatalf("id/timestamps: %+v", p)
	}
	if _, err := domain.NewParticipant(callID, uid, domain.Role("spectator")); !errors.Is(err, domain.ErrInvalidRole) {
		t.Fatalf("bad role: %v", err)
	}
	if _, err := domain.NewParticipant(uuid.Nil, uid, domain.RolePublisher); !errors.Is(err, domain.ErrInvalidCall) {
		t.Fatalf("nil call: %v", err)
	}
}

func TestRoleStatusValid(t *testing.T) {
	if !domain.RolePublisher.Valid() || !domain.RoleViewer.Valid() || domain.Role("x").Valid() {
		t.Fatal("Role.Valid")
	}
	if !domain.StatusActive.Valid() || !domain.StatusEnded.Valid() || domain.Status("x").Valid() {
		t.Fatal("Status.Valid")
	}
}
