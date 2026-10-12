// internal/conversation/adapter/out/messaging/inbox_projector_test.go
package messaging

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	"pet-messenger/internal/conversation/adapter/out/persistence"
	"pet-messenger/internal/conversation/domain"
)

// spyInboxWriter is an in-memory inboxWriter that records every call for
// assertions.
type spyInboxWriter struct {
	mu       sync.Mutex
	upserted []persistence.InboxRow
	removed  []removedCall
	archived []archivedCall
}

type removedCall struct {
	conversationID, userID uuid.UUID
	seq                    int64
}

type archivedCall struct {
	conversationID uuid.UUID
	archivedAt     *time.Time
	seq            int64
}

func (s *spyInboxWriter) UpsertMember(_ context.Context, row persistence.InboxRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, row)
	return nil
}

func (s *spyInboxWriter) RemoveMember(_ context.Context, conversationID, userID uuid.UUID, eventSeq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed = append(s.removed, removedCall{conversationID, userID, eventSeq})
	return nil
}

func (s *spyInboxWriter) UpdateArchived(_ context.Context, conversationID uuid.UUID, archivedAt *time.Time, eventSeq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archived = append(s.archived, archivedCall{conversationID, archivedAt, eventSeq})
	return nil
}

func (s *spyInboxWriter) UpdateLastMessageAt(context.Context, uuid.UUID, time.Time) error { return nil }

func (s *spyInboxWriter) snapshotUpserted() []persistence.InboxRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]persistence.InboxRow(nil), s.upserted...)
}

func (s *spyInboxWriter) snapshotRemoved() []removedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]removedCall(nil), s.removed...)
}

func (s *spyInboxWriter) snapshotArchived() []archivedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]archivedCall(nil), s.archived...)
}

// subscribeProjector opens a queue-group subscription for the projector
// under test and returns it, cleaned up via t.Cleanup.
func subscribeProjector(t *testing.T, projector *InboxProjector, group string) {
	t.Helper()
	sub, err := testJS.QueueSubscribe(WildcardSubject, group, projector.HandleLifecycleEvent,
		natsgo.Durable(group), natsgo.ManualAck(), natsgo.AckWait(5*time.Second))
	if err != nil {
		t.Fatalf("QueueSubscribe() error = %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
}

func TestInboxProjector_AppliesMemberAdded(t *testing.T) {
	purgeStream(t)
	pub := NewEventPublisher(testJS)
	spy := &spyInboxWriter{}
	projector := NewInboxProjector(spy)
	subscribeProjector(t, projector, "test-member-added")

	convID, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	ev := domain.MemberAdded{ConversationID: convID, UserID: userID, Kind: domain.KindGroupChat, Title: "Team"}
	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if rows := spy.snapshotUpserted(); len(rows) == 1 {
			if rows[0].ConversationID != convID || rows[0].UserID != userID || rows[0].Title != "Team" {
				t.Fatalf("got row %+v", rows[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for projector to apply MemberAdded")
}

func TestInboxProjector_AppliesMemberRemoved(t *testing.T) {
	purgeStream(t)
	pub := NewEventPublisher(testJS)
	spy := &spyInboxWriter{}
	projector := NewInboxProjector(spy)
	subscribeProjector(t, projector, "test-member-removed")

	convID, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	ev := domain.MemberRemoved{ConversationID: convID, UserID: userID}
	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if calls := spy.snapshotRemoved(); len(calls) == 1 {
			if calls[0].conversationID != convID || calls[0].userID != userID {
				t.Fatalf("got call %+v", calls[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for projector to apply MemberRemoved")
}

func TestInboxProjector_AppliesConversationArchived(t *testing.T) {
	purgeStream(t)
	pub := NewEventPublisher(testJS)
	spy := &spyInboxWriter{}
	projector := NewInboxProjector(spy)
	subscribeProjector(t, projector, "test-archived")

	convID := uuid.Must(uuid.NewV7())
	ev := domain.ConversationArchived{ConversationID: convID}
	if err := pub.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if calls := spy.snapshotArchived(); len(calls) == 1 {
			if calls[0].conversationID != convID || calls[0].archivedAt == nil {
				t.Fatalf("got call %+v", calls[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for projector to apply ConversationArchived")
}

func TestInboxProjector_IgnoresUnconsumedEventTypes(t *testing.T) {
	purgeStream(t)
	pub := NewEventPublisher(testJS)
	spy := &spyInboxWriter{}
	projector := NewInboxProjector(spy)
	subscribeProjector(t, projector, "test-ignored")

	if err := pub.Publish(context.Background(), domain.ConversationCreated{ConversationID: uuid.Must(uuid.NewV7())}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	// Give the subscriber a moment to (not) act, then assert nothing was recorded.
	time.Sleep(200 * time.Millisecond)
	if len(spy.snapshotUpserted()) != 0 || len(spy.snapshotRemoved()) != 0 || len(spy.snapshotArchived()) != 0 {
		t.Fatal("ConversationCreated must not be applied to the inbox projection")
	}
}
