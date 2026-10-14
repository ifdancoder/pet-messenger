// internal/message/application/command/post_system_event_test.go
package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
)

func TestPostSystemEvent_KindGuard(t *testing.T) {
	writer := &fakeMessageWriter{}
	pub := &fakePublisher{}
	h := NewPostSystemEventHandler(writer, pub, &fakeNotifProducer{})
	ctx := context.Background()
	actor := uuid.Must(uuid.NewV7())

	// member_* in a broadcast channel -> no-op
	bc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelBroadcast}
	if err := h.PostSystemEvent(ctx, bc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("broadcast member_joined: %v", err)
	}
	if len(pub.created) != 0 {
		t.Fatal("member_joined must not post in a broadcast channel")
	}

	// member_* in a group chat -> posts
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	if err := h.PostSystemEvent(ctx, gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("group member_joined: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0].SystemEvent != domainmsg.SystemMemberJoined {
		t.Fatalf("created = %+v", pub.created)
	}

	// conversation_archived in a broadcast channel -> posts
	if err := h.PostSystemEvent(ctx, bc, &actor, domainmsg.SystemConversationArchived, uuid.Nil, ""); err != nil {
		t.Fatalf("broadcast archived: %v", err)
	}
	if len(pub.created) != 2 {
		t.Fatalf("conversation_archived should post in a broadcast channel")
	}

	// direct -> no-op
	dc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	if err := h.PostSystemEvent(ctx, dc, &actor, domainmsg.SystemConversationArchived, uuid.Nil, ""); err != nil {
		t.Fatalf("direct: %v", err)
	}
	if len(pub.created) != 2 {
		t.Fatal("nothing posts in a direct conversation")
	}

	// member_* in a channel_group -> posts (only channel_broadcast suppresses it)
	cg := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelGroup}
	if err := h.PostSystemEvent(ctx, cg, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("channel_group member_joined: %v", err)
	}
	if len(pub.created) != 3 || pub.created[2].SystemEvent != domainmsg.SystemMemberJoined {
		t.Fatalf("channel_group member_joined must post; created = %+v", pub.created)
	}
}

func TestPostSystemEvent_NotifiesMembershipEvent(t *testing.T) {
	notifProd := &fakeNotifProducer{}
	h := NewPostSystemEventHandler(&fakeMessageWriter{}, &fakePublisher{}, notifProd)
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())

	// a kick emits one membership-event intent
	if err := h.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberRemoved, target, ""); err != nil {
		t.Fatalf("kick: %v", err)
	}
	// a plain join does not
	if err := h.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, target, ""); err != nil {
		t.Fatalf("join: %v", err)
	}
	if notifProd.membershipEvent != 1 {
		t.Fatalf("membershipEvent = %d, want 1 (kick only)", notifProd.membershipEvent)
	}
}

func TestPostSystemEvent_CallEventInDirect(t *testing.T) {
	pub := &fakePublisher{}
	h := NewPostSystemEventHandler(&fakeMessageWriter{}, pub, &fakeNotifProducer{})
	direct := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect}
	if err := h.PostSystemEvent(context.Background(), direct, nil, domainmsg.SystemCallStarted, uuid.Must(uuid.NewV7()), ""); err != nil {
		t.Fatalf("PostSystemEvent: %v", err)
	}
	if len(pub.created) != 1 || pub.created[0].SystemEvent != domainmsg.SystemCallStarted {
		t.Fatalf("expected one call_started system message, got %+v", pub.created)
	}
}

func TestPostSystemEvent_PersistErrorReturned(t *testing.T) {
	writer := &fakeMessageWriter{createErr: errors.New("db down")}
	h := NewPostSystemEventHandler(writer, &fakePublisher{}, &fakeNotifProducer{})
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	if err := h.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err == nil {
		t.Fatal("want a non-nil error when writer.Create fails")
	}
}

func TestPostSystemEvent_PublishErrorSwallowed(t *testing.T) {
	pub := &fakePublisher{err: errors.New("nats down")}
	h := NewPostSystemEventHandler(&fakeMessageWriter{}, pub, &fakeNotifProducer{})
	gc := &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat}
	actor := uuid.Must(uuid.NewV7())
	if err := h.PostSystemEvent(context.Background(), gc, &actor, domainmsg.SystemMemberJoined, actor, ""); err != nil {
		t.Fatalf("a publish error must be swallowed, got %v", err)
	}
	if len(pub.created) != 1 {
		t.Fatalf("the event should still have been persisted + publish attempted: %+v", pub.created)
	}
}
