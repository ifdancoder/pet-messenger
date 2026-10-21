// internal/notification/application/command/process_notification_intent_test.go
package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/platform/infrastructure/push"
	"pet-messenger/internal/notification/application/port/in"
	domainnotif "pet-messenger/internal/notification/domain"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

type fakeWriter struct {
	created []*domainnotif.Notification
	dedup   map[string]bool // "userID|dedupKey" already created
}

func (f *fakeWriter) Create(_ context.Context, n *domainnotif.Notification) error {
	if n.DedupKey != "" {
		key := n.UserID.String() + "|" + n.DedupKey
		if f.dedup[key] {
			return domainnotif.ErrDuplicate
		}
		if f.dedup == nil {
			f.dedup = map[string]bool{}
		}
		f.dedup[key] = true
	}
	f.created = append(f.created, n)
	return nil
}

func (f *fakeWriter) createdFor(uid uuid.UUID) int {
	n := 0
	for _, c := range f.created {
		if c.UserID == uid {
			n++
		}
	}
	return n
}

type fakeMembers struct {
	ids []uuid.UUID
	err error
}

func (f *fakeMembers) ListMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.ids, f.err
}

type fakePresence struct {
	online map[uuid.UUID]bool
}

func (f *fakePresence) Online(_ context.Context, uid uuid.UUID) (bool, error) {
	return f.online[uid], nil
}

type fakeWS struct {
	notified []uuid.UUID
}

func (f *fakeWS) Notify(uid uuid.UUID, _ []byte) error {
	f.notified = append(f.notified, uid)
	return nil
}

type fakeTokens struct {
	byUser  map[uuid.UUID][]*domainpush.PushToken
	deleted []string
}

func (f *fakeTokens) ListForUser(_ context.Context, uid uuid.UUID) ([]*domainpush.PushToken, error) {
	return f.byUser[uid], nil
}
func (f *fakeTokens) DeleteToken(_ context.Context, token string) error {
	f.deleted = append(f.deleted, token)
	return nil
}

type fakePusher struct {
	gone map[string]bool
	sent []string
}

func (f *fakePusher) Send(_ context.Context, tok *domainpush.PushToken, _ *domainnotif.Notification) error {
	f.sent = append(f.sent, tok.Token)
	if f.gone[tok.Token] {
		return push.ErrTokenGone
	}
	return nil
}

func newHandler(writer *fakeWriter, members *fakeMembers, presence *fakePresence, ws *fakeWS, tokens *fakeTokens, pusher *fakePusher) in.ProcessNotificationIntentUseCase {
	return NewProcessNotificationIntentHandler(writer, members, presence, ws, tokens, pusher)
}

func strptr(s string) *string { return &s }
func i64ptr(v int64) *int64   { return &v }

func TestProcessNotificationIntent_GroupChat_FansOutExcludingSender(t *testing.T) {
	a, b, c := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{ids: []uuid.UUID{a, b, c}}
	presence := &fakePresence{online: map[uuid.UUID]bool{b: true}}
	ws := &fakeWS{}
	tokens := &fakeTokens{byUser: map[uuid.UUID][]*domainpush.PushToken{
		c: {{Token: "c-tok-1"}, {Token: "c-tok-2"}},
	}}
	pusher := &fakePusher{}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	convID := uuid.Must(uuid.NewV7()).String()
	msgID := uuid.Must(uuid.NewV7()).String()
	cmd := in.ProcessNotificationIntentCommand{
		Kind: "message", ConversationID: convID,
		MessageID: strptr(msgID), MessageSeq: i64ptr(7), SenderID: strptr(a.String()),
		Preview: "hi there", DedupPrefix: "msg",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if writer.createdFor(a) != 0 {
		t.Errorf("sender rows = %d, want 0", writer.createdFor(a))
	}
	if writer.createdFor(b) != 1 {
		t.Errorf("B rows = %d, want 1", writer.createdFor(b))
	}
	if writer.createdFor(c) != 1 {
		t.Errorf("C rows = %d, want 1", writer.createdFor(c))
	}
	if len(ws.notified) != 1 || ws.notified[0] != b {
		t.Errorf("ws notified = %v, want [B]", ws.notified)
	}
	if len(pusher.sent) != 2 {
		t.Errorf("pusher calls = %v, want 2 (C's tokens)", pusher.sent)
	}
}

func TestProcessNotificationIntent_Redeliver_NoDuplicateRows(t *testing.T) {
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{ids: []uuid.UUID{a, b}}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{byUser: map[uuid.UUID][]*domainpush.PushToken{b: {{Token: "b-tok"}}}}
	pusher := &fakePusher{}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	msgID := uuid.Must(uuid.NewV7()).String()
	cmd := in.ProcessNotificationIntentCommand{
		Kind: "message", ConversationID: uuid.Must(uuid.NewV7()).String(),
		MessageID: strptr(msgID), MessageSeq: i64ptr(1), SenderID: strptr(a.String()),
		Preview: "x", DedupPrefix: "msg",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle (first): %v", err)
	}
	if writer.createdFor(b) != 1 {
		t.Fatalf("after first handle: B rows = %d, want 1", writer.createdFor(b))
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle (redeliver): %v", err)
	}
	if writer.createdFor(b) != 1 {
		t.Errorf("after redeliver: B rows = %d, want 1 (dedup)", writer.createdFor(b))
	}
}

func TestProcessNotificationIntent_GoneToken_Deleted(t *testing.T) {
	a, c := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{ids: []uuid.UUID{a, c}}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{byUser: map[uuid.UUID][]*domainpush.PushToken{
		c: {{Token: "gone-tok"}, {Token: "live-tok"}},
	}}
	pusher := &fakePusher{gone: map[string]bool{"gone-tok": true}}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	msgID := uuid.Must(uuid.NewV7()).String()
	cmd := in.ProcessNotificationIntentCommand{
		Kind: "message", ConversationID: uuid.Must(uuid.NewV7()).String(),
		MessageID: strptr(msgID), MessageSeq: i64ptr(3), SenderID: strptr(a.String()),
		Preview: "y", DedupPrefix: "msg",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(tokens.deleted) != 1 || tokens.deleted[0] != "gone-tok" {
		t.Errorf("deleted = %v, want [gone-tok]", tokens.deleted)
	}
	if writer.createdFor(c) != 1 {
		t.Errorf("C rows = %d, want 1", writer.createdFor(c))
	}
}

func TestProcessNotificationIntent_MemberRemoved_SingleTarget(t *testing.T) {
	d := uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{byUser: map[uuid.UUID][]*domainpush.PushToken{d: {{Token: "d-tok"}}}}
	pusher := &fakePusher{}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	cmd := in.ProcessNotificationIntentCommand{
		Kind: "member_removed", ConversationID: uuid.Must(uuid.NewV7()).String(),
		RecipientID: strptr(d.String()), DedupPrefix: "kick",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if writer.createdFor(d) != 1 {
		t.Errorf("D rows = %d, want 1", writer.createdFor(d))
	}
	if len(pusher.sent) != 1 || pusher.sent[0] != "d-tok" {
		t.Errorf("pusher calls = %v, want [d-tok]", pusher.sent)
	}
}

func TestProcessNotificationIntent_CallStarted_FansOutAndDedups(t *testing.T) {
	a, b, c := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{ids: []uuid.UUID{a, b, c}}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{}
	pusher := &fakePusher{}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	callID := uuid.Must(uuid.NewV7()).String()
	cmd := in.ProcessNotificationIntentCommand{
		Kind: "call_started", ConversationID: uuid.Must(uuid.NewV7()).String(),
		SenderID: strptr(a.String()), CallID: callID, DedupPrefix: "call",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle (first): %v", err)
	}
	for _, uid := range []uuid.UUID{b, c} {
		if writer.createdFor(uid) != 1 {
			t.Errorf("member %s rows = %d, want 1", uid, writer.createdFor(uid))
		}
	}
	if writer.createdFor(a) != 0 {
		t.Errorf("initiator rows = %d, want 0", writer.createdFor(a))
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle (redeliver): %v", err)
	}
	for _, uid := range []uuid.UUID{b, c} {
		if writer.createdFor(uid) != 1 {
			t.Errorf("member %s rows after redeliver = %d, want 1 (dedup)", uid, writer.createdFor(uid))
		}
	}
}

func TestProcessNotificationIntent_UnknownKind_NoRowNoError(t *testing.T) {
	d := uuid.Must(uuid.NewV7())
	writer := &fakeWriter{}
	members := &fakeMembers{}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{byUser: map[uuid.UUID][]*domainpush.PushToken{d: {{Token: "garbage-tok"}}}}
	pusher := &fakePusher{}
	h := newHandler(writer, members, presence, ws, tokens, pusher)

	cmd := in.ProcessNotificationIntentCommand{
		Kind: "garbage", ConversationID: uuid.Must(uuid.NewV7()).String(),
		RecipientID: strptr(d.String()), DedupPrefix: "x",
	}
	if err := h.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if writer.createdFor(d) != 0 {
		t.Errorf("unknown-kind wrote %d rows, want 0", writer.createdFor(d))
	}
	if len(pusher.sent) != 0 {
		t.Errorf("pusher called %v, want none", pusher.sent)
	}
}

func TestProcessNotificationIntent_WriterTransientError_Propagates(t *testing.T) {
	b := uuid.Must(uuid.NewV7())
	members := &fakeMembers{ids: []uuid.UUID{b}}
	presence := &fakePresence{}
	ws := &fakeWS{}
	tokens := &fakeTokens{}
	pusher := &fakePusher{}
	h := NewProcessNotificationIntentHandler(&erroringWriter{}, members, presence, ws, tokens, pusher)

	cmd := in.ProcessNotificationIntentCommand{
		Kind: "message", ConversationID: uuid.Must(uuid.NewV7()).String(),
		MessageID: strptr(uuid.Must(uuid.NewV7()).String()), DedupPrefix: "msg",
	}
	if err := h.Handle(context.Background(), cmd); err == nil {
		t.Fatal("Handle: want error, got nil")
	}
}

// TestWireEnvelope_MatchesPushBodyShape pins wireEnvelope's output
// byte-identical (as JSON, modulo key order) to push.Body's: both must
// serialize the exact same wire shape for the same Notification, since one
// feeds the WS delivery path (notif.ws.<uid>) and the other the device-push
// path, and clients on either path expect the same envelope.
func TestWireEnvelope_MatchesPushBodyShape(t *testing.T) {
	n, err := domainnotif.New(uuid.Must(uuid.NewV7()), domainnotif.KindMessage, []byte(`{"a":1}`), "")
	if err != nil {
		t.Fatalf("domainnotif.New: %v", err)
	}

	wsBytes := wireEnvelope(n)
	pushBytes := push.Body(n)

	var wsMap, pushMap map[string]any
	if err := json.Unmarshal(wsBytes, &wsMap); err != nil {
		t.Fatalf("wireEnvelope output is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(pushBytes, &pushMap); err != nil {
		t.Fatalf("push.Body output is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(wsMap, pushMap) {
		t.Fatalf("wireEnvelope and push.Body disagree on wire shape:\n  wireEnvelope=%v\n  push.Body   =%v", wsMap, pushMap)
	}
}

type erroringWriter struct{}

func (erroringWriter) Create(context.Context, *domainnotif.Notification) error {
	return errors.New("boom")
}
