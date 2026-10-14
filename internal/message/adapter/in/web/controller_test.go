// internal/message/adapter/in/web/controller_test.go
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	domainconv "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/message/application/command"
	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/query"
	domainmsg "pet-messenger/internal/message/domain"
)

type fakeSend struct {
	fn func(actor, convID uuid.UUID, content string, replyTo *uuid.UUID, idem string) (*domainmsg.Message, error)
}

func (f *fakeSend) Handle(_ context.Context, cmd in.SendMessageCommand) (*domainmsg.Message, error) {
	return f.fn(cmd.Actor, cmd.ConversationID, cmd.Content, cmd.ReplyToID, cmd.IdempotencyKey)
}

type fakeEdit struct {
	fn func(actor, convID, msgID uuid.UUID, content string) (*domainmsg.Message, error)
}

func (f *fakeEdit) Handle(_ context.Context, cmd in.EditMessageCommand) (*domainmsg.Message, error) {
	return f.fn(cmd.Actor, cmd.ConversationID, cmd.MessageID, cmd.Content)
}

type fakeDelete struct {
	fn func(actor, convID, msgID uuid.UUID) (*domainmsg.Message, error)
}

func (f *fakeDelete) Handle(_ context.Context, cmd in.DeleteMessageCommand) (*domainmsg.Message, error) {
	return f.fn(cmd.Actor, cmd.ConversationID, cmd.MessageID)
}

type fakeList struct {
	fn func(actor, convID uuid.UUID, after, before *int64, limit int) ([]*domainmsg.Message, error)
}

func (f *fakeList) Handle(_ context.Context, q in.ListMessagesQuery) ([]*domainmsg.Message, error) {
	return f.fn(q.Actor, q.ConversationID, q.AfterSeq, q.BeforeSeq, q.Limit)
}

type fakeMarkRead struct {
	fn func(actor, convID, messageID uuid.UUID) error
}

func (f *fakeMarkRead) Handle(_ context.Context, cmd in.MarkReadCommand) error {
	if f.fn == nil {
		return nil
	}
	return f.fn(cmd.Actor, cmd.ConversationID, cmd.MessageID)
}

func msgRouter(h *MessageHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/conversations")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(httpadapter.ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func doMsg(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sampleMessage(convID, sender uuid.UUID, seq int64) *domainmsg.Message {
	s := sender
	return &domainmsg.Message{
		ID: uuid.Must(uuid.NewV7()), ConversationID: convID, Seq: seq,
		SenderID: &s, Type: domainmsg.TypeText, Content: "hi", CreatedAt: time.Now().UTC(),
	}
}

func TestMessageHandler_Send_Created(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	reply := uuid.Must(uuid.NewV7())
	want := sampleMessage(convID, actor, 3)
	h := &MessageHandler{send: &fakeSend{fn: func(gotActor, gotConv uuid.UUID, content string, replyTo *uuid.UUID, idem string) (*domainmsg.Message, error) {
		if gotActor != actor || gotConv != convID || content != "hello" || replyTo == nil || *replyTo != reply || idem != "k9" {
			t.Errorf("send args: actor=%v conv=%v content=%q reply=%v idem=%q", gotActor, gotConv, content, replyTo, idem)
		}
		return want, nil
	}}}
	r := msgRouter(h, &actor)

	w := doMsg(t, r, http.MethodPost, "/conversations/"+convID.String()+"/messages",
		`{"content":"hello","reply_to_id":"`+reply.String()+`","idempotency_key":"k9"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/conversations/"+convID.String()+"/messages/"+want.ID.String() {
		t.Errorf("Location = %q", loc)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["id"] != want.ID.String() || body["seq"].(float64) != 3 {
		t.Errorf("body = %v", body)
	}
}

func TestMessageHandler_Send_MissingContent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
		t.Fatal("use case must not be called")
		return nil, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestMessageHandler_Send_IdempotencyKeyTooLong(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
		t.Fatal("use case must not be called")
		return nil, nil
	}}}
	r := msgRouter(h, &actor)
	key := strings.Repeat("a", 256)
	w := doMsg(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages",
		`{"content":"hi","idempotency_key":"`+key+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a 256-char idempotency_key", w.Code)
	}
}

func TestMessageHandler_Send_BadConversationID(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
		t.Fatal("use case must not be called")
		return nil, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodPost, "/conversations/not-a-uuid/messages", `{"content":"hi"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestMessageHandler_Send_BadReplyToID(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
		t.Fatal("use case must not be called")
		return nil, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", `{"content":"hi","reply_to_id":"xxx"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestMessageHandler_Send_NoAuthContext(t *testing.T) {
	h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
		return nil, nil
	}}}
	r := msgRouter(h, nil)
	w := doMsg(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", `{"content":"hi"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestMessageHandler_Send_ErrorMapping(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not member", domainconv.ErrNotMember, http.StatusForbidden},
		{"cannot post", command.ErrCannotPost, http.StatusForbidden},
		{"reply missing", command.ErrReplyNotFound, http.StatusBadRequest},
		{"conversation missing", domainconv.ErrNotFound, http.StatusNotFound},
		{"domain conversation not found", domainmsg.ErrConversationNotFound, http.StatusNotFound},
		{"empty content", domainmsg.ErrEmptyContent, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &MessageHandler{send: &fakeSend{fn: func(uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*domainmsg.Message, error) {
				return nil, tc.err
			}}}
			r := msgRouter(h, &actor)
			w := doMsg(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", `{"content":"hi"}`)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestMessageHandler_List_FullPageHasCursor(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	page := make([]*domainmsg.Message, query.DefaultPageLimit)
	for i := range page {
		page[i] = sampleMessage(convID, actor, int64(query.DefaultPageLimit-i)) // descending
	}
	h := &MessageHandler{list: &fakeList{fn: func(_, _ uuid.UUID, _, _ *int64, _ int) ([]*domainmsg.Message, error) {
		return page, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodGet, "/conversations/"+convID.String()+"/messages", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Messages []map[string]any `json:"messages"`
		NextSeq  int64            `json:"next_seq"`
		HasMore  bool             `json:"has_more"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Messages) != query.DefaultPageLimit || !body.HasMore {
		t.Fatalf("len=%d has_more=%v", len(body.Messages), body.HasMore)
	}
	if body.NextSeq != page[len(page)-1].Seq {
		t.Errorf("next_seq = %d, want %d", body.NextSeq, page[len(page)-1].Seq)
	}
}

func TestMessageHandler_List_ShortPageNoCursor(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	h := &MessageHandler{list: &fakeList{fn: func(_, _ uuid.UUID, _, _ *int64, _ int) ([]*domainmsg.Message, error) {
		return []*domainmsg.Message{sampleMessage(convID, actor, 1)}, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodGet, "/conversations/"+convID.String()+"/messages", "")
	var body struct {
		NextSeq int64 `json:"next_seq"`
		HasMore bool  `json:"has_more"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.HasMore || body.NextSeq != 0 {
		t.Errorf("short page: has_more=%v next_seq=%d, want false/0", body.HasMore, body.NextSeq)
	}
}

func TestMessageHandler_List_CursorsReachUseCase(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	var gotAfter, gotBefore *int64
	var gotLimit int
	h := &MessageHandler{list: &fakeList{fn: func(_, _ uuid.UUID, after, before *int64, limit int) ([]*domainmsg.Message, error) {
		gotAfter, gotBefore, gotLimit = after, before, limit
		return nil, nil
	}}}
	r := msgRouter(h, &actor)

	w := doMsg(t, r, http.MethodGet, "/conversations/"+convID.String()+"/messages?after_seq=12&limit=25", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if gotAfter == nil || *gotAfter != 12 || gotBefore != nil || gotLimit != 25 {
		t.Errorf("after=%v before=%v limit=%d", gotAfter, gotBefore, gotLimit)
	}
}

func TestMessageHandler_List_BadAfterSeq(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{list: &fakeList{fn: func(uuid.UUID, uuid.UUID, *int64, *int64, int) ([]*domainmsg.Message, error) {
		t.Fatal("use case must not be called")
		return nil, nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodGet, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages?after_seq=abc", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestMessageHandler_List_ConflictingCursors(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{list: &fakeList{fn: func(uuid.UUID, uuid.UUID, *int64, *int64, int) ([]*domainmsg.Message, error) {
		return nil, query.ErrConflictingCursors
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodGet, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages?after_seq=1&before_seq=2", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestMessageHandler_List_NotMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := &MessageHandler{list: &fakeList{fn: func(uuid.UUID, uuid.UUID, *int64, *int64, int) ([]*domainmsg.Message, error) {
		return nil, domainconv.ErrNotMember
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodGet, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/messages", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestMessageHandler_Edit(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())

	t.Run("ok", func(t *testing.T) {
		want := sampleMessage(convID, actor, 4)
		want.Content = "edited"
		h := &MessageHandler{edit: &fakeEdit{fn: func(gotA, gotC, gotM uuid.UUID, content string) (*domainmsg.Message, error) {
			if gotA != actor || gotC != convID || gotM != msgID || content != "edited" {
				t.Errorf("edit args: %v %v %v %q", gotA, gotC, gotM, content)
			}
			return want, nil
		}}}
		r := msgRouter(h, &actor)
		w := doMsg(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/messages/"+msgID.String(), `{"content":"edited"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad message id", func(t *testing.T) {
		h := &MessageHandler{edit: &fakeEdit{fn: func(uuid.UUID, uuid.UUID, uuid.UUID, string) (*domainmsg.Message, error) {
			t.Fatal("use case must not be called")
			return nil, nil
		}}}
		r := msgRouter(h, &actor)
		w := doMsg(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/messages/nope", `{"content":"x"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"not author", command.ErrNotAuthor, http.StatusForbidden},
		{"not found", domainmsg.ErrNotFound, http.StatusNotFound},
		{"not editable", domainmsg.ErrNotEditable, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &MessageHandler{edit: &fakeEdit{fn: func(uuid.UUID, uuid.UUID, uuid.UUID, string) (*domainmsg.Message, error) {
				return nil, tc.err
			}}}
			r := msgRouter(h, &actor)
			w := doMsg(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/messages/"+msgID.String(), `{"content":"x"}`)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestMessageHandler_Delete(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())

	t.Run("ok returns tombstone", func(t *testing.T) {
		tomb := sampleMessage(convID, actor, 2)
		tomb.Content = ""
		now := time.Now().UTC()
		tomb.DeletedAt = &now
		h := &MessageHandler{del: &fakeDelete{fn: func(uuid.UUID, uuid.UUID, uuid.UUID) (*domainmsg.Message, error) {
			return tomb, nil
		}}}
		r := msgRouter(h, &actor)
		w := doMsg(t, r, http.MethodDelete, "/conversations/"+convID.String()+"/messages/"+msgID.String(), "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["content"] != "" || body["deleted_at"] == nil {
			t.Errorf("body = %v, want cleared content + deleted_at set", body)
		}
	})

	t.Run("not author", func(t *testing.T) {
		h := &MessageHandler{del: &fakeDelete{fn: func(uuid.UUID, uuid.UUID, uuid.UUID) (*domainmsg.Message, error) {
			return nil, command.ErrNotAuthor
		}}}
		r := msgRouter(h, &actor)
		w := doMsg(t, r, http.MethodDelete, "/conversations/"+convID.String()+"/messages/"+msgID.String(), "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})
}

func TestMessageHandler_MarkRead(t *testing.T) {
	actor, convID, messageID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	called := false
	h := &MessageHandler{markRead: &fakeMarkRead{fn: func(gotActor, gotConversation, gotMessage uuid.UUID) error {
		called = gotActor == actor && gotConversation == convID && gotMessage == messageID
		return nil
	}}}
	r := msgRouter(h, &actor)
	w := doMsg(t, r, http.MethodPost, "/conversations/"+convID.String()+"/read", `{"message_id":"`+messageID.String()+`"}`)
	if w.Code != http.StatusNoContent || !called {
		t.Fatalf("POST read status=%d called=%t, want 204 and use-case call", w.Code, called)
	}
}
