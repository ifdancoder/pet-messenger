package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	domainmsg "pet-messenger/internal/domain/message"
	usecaseconv "pet-messenger/internal/usecase/conversation"
)

type fakeConversationService struct {
	openDirect         func(me, other uuid.UUID) (*domainconv.Conversation, error)
	createGroup        func(creator uuid.UUID, title string) (*domainconv.Conversation, error)
	listMine           func(me uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
	createChannel      func(creator uuid.UUID, kind domainconv.Kind, title string, public bool) (*domainconv.Conversation, error)
	joinChannel        func(actor, channelID uuid.UUID) (*domainconv.Conversation, error)
	leaveChannel       func(actor, channelID uuid.UUID) error
	listPublicChannels func(query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
}

func (f *fakeConversationService) OpenDirect(_ context.Context, me, other uuid.UUID) (*domainconv.Conversation, error) {
	return f.openDirect(me, other)
}
func (f *fakeConversationService) CreateGroupChat(_ context.Context, creator uuid.UUID, title string) (*domainconv.Conversation, error) {
	return f.createGroup(creator, title)
}
func (f *fakeConversationService) ListMine(_ context.Context, me uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	return f.listMine(me, before, beforeID, limit)
}
func (f *fakeConversationService) CreateChannel(_ context.Context, creator uuid.UUID, kind domainconv.Kind, title string, public bool) (*domainconv.Conversation, error) {
	return f.createChannel(creator, kind, title, public)
}
func (f *fakeConversationService) JoinChannel(_ context.Context, actor, channelID uuid.UUID) (*domainconv.Conversation, error) {
	return f.joinChannel(actor, channelID)
}
func (f *fakeConversationService) LeaveChannel(_ context.Context, actor, channelID uuid.UUID) error {
	return f.leaveChannel(actor, channelID)
}
func (f *fakeConversationService) ListPublicChannels(_ context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	return f.listPublicChannels(query, before, beforeID, limit)
}

func convRouter(h *ConversationHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/conversations")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func doConvJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func newDirectConv() *domainconv.Conversation {
	return &domainconv.Conversation{
		ID:            uuid.Must(uuid.NewV7()),
		Kind:          domainconv.KindDirect,
		CreatedAt:     time.Now().UTC(),
		LastMessageAt: time.Now().UTC(),
	}
}

func TestConversationHandler_OpenDirect_OK(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	want := newDirectConv()
	h := &ConversationHandler{svc: &fakeConversationService{
		openDirect: func(gotMe, gotOther uuid.UUID) (*domainconv.Conversation, error) {
			if gotMe != me || gotOther != other {
				t.Errorf("OpenDirect(%v,%v), want (%v,%v)", gotMe, gotOther, me, other)
			}
			return want, nil
		},
	}}
	r := convRouter(h, &me)

	w := doConvJSON(t, r, http.MethodPost, "/conversations/direct", `{"user_id":"`+other.String()+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["id"] != want.ID.String() || body["kind"] != "direct" {
		t.Errorf("body = %v", body)
	}
}

func TestConversationHandler_OpenDirect_BadUUID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		openDirect: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) {
			t.Fatal("service must not be called on a bad user_id")
			return nil, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/direct", `{"user_id":"not-a-uuid"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_OpenDirect_MissingField(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		openDirect: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) { return nil, nil },
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/direct", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_OpenDirect_ErrorMapping(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"self", usecaseconv.ErrCannotDirectSelf, http.StatusBadRequest},
		{"invalid members", domainconv.ErrInvalidDirectMembers, http.StatusBadRequest},
		{"recipient missing", usecaseconv.ErrRecipientNotFound, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &ConversationHandler{svc: &fakeConversationService{
				openDirect: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) { return nil, tc.err },
			}}
			r := convRouter(h, &me)
			w := doConvJSON(t, r, http.MethodPost, "/conversations/direct", `{"user_id":"`+other.String()+`"}`)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestConversationHandler_OpenDirect_NoAuthContext(t *testing.T) {
	h := &ConversationHandler{svc: &fakeConversationService{
		openDirect: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) { return nil, nil },
	}}
	r := convRouter(h, nil) // no user set on the context
	w := doConvJSON(t, r, http.MethodPost, "/conversations/direct", `{"user_id":"`+uuid.Must(uuid.NewV7()).String()+`"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestConversationHandler_CreateGroupChat_Created(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	want := &domainconv.Conversation{
		ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat, Title: "Team",
		CreatedAt: time.Now().UTC(), LastMessageAt: time.Now().UTC(),
	}
	h := &ConversationHandler{svc: &fakeConversationService{
		createGroup: func(creator uuid.UUID, title string) (*domainconv.Conversation, error) {
			if creator != me || title != "Team" {
				t.Errorf("CreateGroupChat(%v, %q)", creator, title)
			}
			return want, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/groups", `{"title":"Team"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/conversations/"+want.ID.String() {
		t.Errorf("Location = %q", loc)
	}
}

func TestConversationHandler_CreateGroupChat_InvalidTitle(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		createGroup: func(uuid.UUID, string) (*domainconv.Conversation, error) {
			return nil, domainconv.ErrInvalidTitle
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/groups", `{"title":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_ListMine_OKWithNextCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	full := make([]*domainconv.Conversation, usecaseconv.DefaultListLimit)
	base := time.Now().UTC()
	for i := range full {
		full[i] = &domainconv.Conversation{
			ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindGroupChat,
			CreatedAt: base, LastMessageAt: base.Add(-time.Duration(i) * time.Minute),
		}
	}
	h := &ConversationHandler{svc: &fakeConversationService{
		listMine: func(_ uuid.UUID, _ time.Time, _ uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
			return full, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Conversations []map[string]any `json:"conversations"`
		NextCursor    string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Conversations) != usecaseconv.DefaultListLimit {
		t.Errorf("len = %d, want %d", len(body.Conversations), usecaseconv.DefaultListLimit)
	}
	if body.NextCursor == "" {
		t.Error("want a non-empty next_cursor on a full page")
	}
}

func TestConversationHandler_ListMine_ShortPageNoCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		listMine: func(_ uuid.UUID, _ time.Time, _ uuid.UUID, _ int) ([]*domainconv.Conversation, error) {
			return []*domainconv.Conversation{{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect, LastMessageAt: time.Now().UTC()}}, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations", "")
	var body struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty on a short page", body.NextCursor)
	}
}

func TestConversationHandler_ListMine_BadCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		listMine: func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainconv.Conversation, error) {
			t.Fatal("service must not be called on a bad cursor")
			return nil, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations?cursor=%21%21%21", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_ListMine_CursorAndLimitReachService(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	wantTS := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	wantID := uuid.Must(uuid.NewV7())
	var gotBefore time.Time
	var gotBeforeID uuid.UUID
	var gotLimit int
	h := &ConversationHandler{svc: &fakeConversationService{
		listMine: func(_ uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
			gotBefore, gotBeforeID, gotLimit = before, beforeID, limit
			return nil, nil
		},
	}}
	r := convRouter(h, &me)
	cursor := encodeConversationCursor(wantTS, wantID)
	w := doConvJSON(t, r, http.MethodGet, "/conversations?limit=7&cursor="+cursor, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !gotBefore.Equal(wantTS) || gotBeforeID != wantID || gotLimit != 7 {
		t.Errorf("service got (before=%v, id=%v, limit=%d)", gotBefore, gotBeforeID, gotLimit)
	}
}

func channelConv(kind domainconv.Kind) *domainconv.Conversation {
	return &domainconv.Conversation{
		ID: uuid.Must(uuid.NewV7()), Kind: kind, Title: "News", Public: true,
		CreatedAt: time.Now().UTC(), LastMessageAt: time.Now().UTC(),
	}
}

func TestConversationHandler_CreateChannel(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	want := channelConv(domainconv.KindChannelBroadcast)
	h := &ConversationHandler{svc: &fakeConversationService{
		createChannel: func(gotCreator uuid.UUID, kind domainconv.Kind, title string, public bool) (*domainconv.Conversation, error) {
			if gotCreator != me || kind != domainconv.KindChannelBroadcast || title != "News" || !public {
				t.Errorf("CreateChannel args: %v %q %q %v", gotCreator, kind, title, public)
			}
			return want, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/channels", `{"kind":"channel_broadcast","title":"News","public":true}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/conversations/"+want.ID.String() {
		t.Errorf("Location = %q", loc)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["kind"] != "channel_broadcast" || body["public"] != true {
		t.Errorf("body = %v", body)
	}
}

func TestConversationHandler_CreateChannel_MissingKind(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		createChannel: func(uuid.UUID, domainconv.Kind, string, bool) (*domainconv.Conversation, error) {
			t.Fatal("service must not be called")
			return nil, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/channels", `{"title":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_CreateChannel_BadKindMapsTo400(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		createChannel: func(uuid.UUID, domainconv.Kind, string, bool) (*domainconv.Conversation, error) {
			return nil, domainconv.ErrInvalidKind
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/channels", `{"kind":"group_chat","title":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_JoinChannel(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	want := channelConv(domainconv.KindChannelGroup)
	h := &ConversationHandler{svc: &fakeConversationService{
		joinChannel: func(actor, channelID uuid.UUID) (*domainconv.Conversation, error) {
			if actor != me || channelID != want.ID {
				t.Errorf("JoinChannel(%v, %v)", actor, channelID)
			}
			return want, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+want.ID.String()+"/join", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["id"] != want.ID.String() {
		t.Errorf("body = %v", body)
	}
}

func TestConversationHandler_JoinChannel_ErrorMapping(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"not a channel", domainconv.ErrNotChannel, http.StatusNotFound},
		{"not found", domainconv.ErrNotFound, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &ConversationHandler{svc: &fakeConversationService{
				joinChannel: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) { return nil, tc.err },
			}}
			r := convRouter(h, &me)
			w := doConvJSON(t, r, http.MethodPost, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/join", "")
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestConversationHandler_JoinChannel_BadID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		joinChannel: func(uuid.UUID, uuid.UUID) (*domainconv.Conversation, error) {
			t.Fatal("service must not be called")
			return nil, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/not-a-uuid/join", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConversationHandler_LeaveChannel(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		leaveChannel: func(actor, channelID uuid.UUID) error {
			if actor != me || channelID != chID {
				t.Errorf("LeaveChannel(%v, %v)", actor, channelID)
			}
			return nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodDelete, "/conversations/"+chID.String()+"/members/me", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestConversationHandler_LeaveChannel_NotMember(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		leaveChannel: func(uuid.UUID, uuid.UUID) error { return domainconv.ErrNotMember },
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodDelete, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/members/me", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestConversationHandler_ListPublicChannels(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	full := make([]*domainconv.Conversation, usecaseconv.DefaultListLimit)
	base := time.Now().UTC()
	for i := range full {
		full[i] = &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelGroup, Public: true, LastMessageAt: base.Add(-time.Duration(i) * time.Minute)}
	}
	var gotQuery string
	var gotLimit int
	h := &ConversationHandler{svc: &fakeConversationService{
		listPublicChannels: func(query string, _ time.Time, _ uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
			gotQuery, gotLimit = query, limit
			return full, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations/channels?q=news&limit=20", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if gotQuery != "news" || gotLimit != 20 {
		t.Errorf("service got q=%q limit=%d", gotQuery, gotLimit)
	}
	var body struct {
		Channels   []map[string]any `json:"channels"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Channels) != usecaseconv.DefaultListLimit || body.NextCursor == "" {
		t.Fatalf("len=%d next_cursor=%q (want a full page + a cursor)", len(body.Channels), body.NextCursor)
	}
}

func TestConversationHandler_ListPublicChannels_ShortPageNoCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		listPublicChannels: func(string, time.Time, uuid.UUID, int) ([]*domainconv.Conversation, error) {
			return []*domainconv.Conversation{{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindChannelGroup, Public: true, LastMessageAt: time.Now().UTC()}}, nil
		},
	}}
	r := convRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations/channels", "")
	var body struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty on a short page", body.NextCursor)
	}
}

func TestConversationHandler_ChannelRoutesCoexist(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convH := &ConversationHandler{svc: &fakeConversationService{
		createChannel: func(uuid.UUID, domainconv.Kind, string, bool) (*domainconv.Conversation, error) {
			return channelConv(domainconv.KindChannelGroup), nil
		},
		joinChannel: func(_, id uuid.UUID) (*domainconv.Conversation, error) {
			return &domainconv.Conversation{ID: id, Kind: domainconv.KindChannelGroup, Public: true, LastMessageAt: time.Now().UTC()}, nil
		},
		openDirect: func(_, _ uuid.UUID) (*domainconv.Conversation, error) {
			return &domainconv.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domainconv.KindDirect, LastMessageAt: time.Now().UTC()}, nil
		},
	}}
	msgH := &MessageHandler{svc: &fakeMessageService{
		send: func(_, cv uuid.UUID, _ string, _ *uuid.UUID, _ string) (*domainmsg.Message, error) {
			return sampleMessage(cv, me, 1), nil
		},
	}}
	gin.SetMode(gin.TestMode)
	rt := gin.New()
	grp := rt.Group("/conversations")
	grp.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, me); c.Next() })
	convH.RegisterRoutes(grp)
	msgH.RegisterRoutes(grp)

	cid := uuid.Must(uuid.NewV7())
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/conversations/channels", `{"kind":"channel_group","title":"x"}`},
		{http.MethodPost, "/conversations/direct", `{"user_id":"` + uuid.Must(uuid.NewV7()).String() + `"}`},
		{http.MethodPost, "/conversations/" + cid.String() + "/join", ""},
		{http.MethodPost, "/conversations/" + cid.String() + "/messages", `{"content":"hi"}`},
	} {
		w := doConvJSON(t, rt, tc.method, tc.path, tc.body)
		if w.Code == http.StatusNotFound {
			t.Fatalf("%s %s → 404 (route did not resolve)", tc.method, tc.path)
		}
	}
}
