package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/call"

	conversation "pet-messenger/internal/conversation/domain"
)

func newLiveCall(t *testing.T) (*call.Call, uuid.UUID) {
	t.Helper()
	convID, creatorID := seedGroupChat(t)
	c, err := call.New(convID, creatorID, "node-test")
	if err != nil {
		t.Fatalf("call.New: %v", err)
	}
	if err := NewCallRepository(testDB).Create(context.Background(), c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c, creatorID
}

func TestCallRepository_CreateAndGet(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, _ := newLiveCall(t)

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != c.ID || got.Status != call.StatusActive || got.MediaNodeID != "node-test" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := repo.GetByID(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, call.ErrNotFound) {
		t.Fatalf("missing GetByID: %v", err)
	}
}

func TestCallRepository_OneLivePerConversation(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, creatorID := newLiveCall(t)

	dup, _ := call.New(c.ConversationID, creatorID, "node-test")
	err := repo.Create(ctx, dup)
	if err == nil {
		t.Fatal("second live call for the same conversation must fail")
	}

	live, err := repo.GetLiveForConversation(ctx, c.ConversationID)
	if err != nil || live.ID != c.ID {
		t.Fatalf("GetLiveForConversation = %v, %v", live, err)
	}

	// after the first ends, a new live call is allowed
	if _, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkEnded: %v", err)
	}
	if _, err := repo.GetLiveForConversation(ctx, c.ConversationID); !errors.Is(err, call.ErrNotFound) {
		t.Fatalf("GetLiveForConversation after end: %v", err)
	}
	if err := repo.Create(ctx, dup); err != nil {
		t.Fatalf("Create after previous ended: %v", err)
	}
}

func TestCallRepository_MarkEndedIdempotent(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, _ := newLiveCall(t)

	first, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil || !first {
		t.Fatalf("first MarkEnded = %v, %v (want true, nil)", first, err)
	}
	second, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil || second {
		t.Fatalf("second MarkEnded = %v, %v (want false, nil)", second, err)
	}
}

func TestCallRepository_Participants(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, creatorID := newLiveCall(t)
	other := addMember(t, c.ConversationID, conversation.TierNone, time.Now().UTC())

	p1, _ := call.NewParticipant(c.ID, creatorID, call.RolePublisher)
	if err := repo.AddParticipant(ctx, p1); err != nil {
		t.Fatalf("AddParticipant p1: %v", err)
	}
	dup, _ := call.NewParticipant(c.ID, creatorID, call.RoleViewer)
	if err := repo.AddParticipant(ctx, dup); !errors.Is(err, call.ErrAlreadyInCall) {
		t.Fatalf("dup AddParticipant: %v, want ErrAlreadyInCall", err)
	}
	p2, _ := call.NewParticipant(c.ID, other, call.RoleViewer)
	if err := repo.AddParticipant(ctx, p2); err != nil {
		t.Fatalf("AddParticipant p2: %v", err)
	}

	list, err := repo.ListParticipants(ctx, c.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListParticipants = %d, %v", len(list), err)
	}

	if err := repo.MarkParticipantLeft(ctx, c.ID, creatorID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkParticipantLeft: %v", err)
	}
	// idempotent: a second call (no active row) is fine
	if err := repo.MarkParticipantLeft(ctx, c.ID, creatorID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkParticipantLeft again: %v", err)
	}
	// creator may re-join now that the active row is closed
	rejoin, _ := call.NewParticipant(c.ID, creatorID, call.RolePublisher)
	if err := repo.AddParticipant(ctx, rejoin); err != nil {
		t.Fatalf("re-join after leave: %v", err)
	}
}
