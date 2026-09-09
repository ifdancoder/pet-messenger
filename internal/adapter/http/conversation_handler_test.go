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
	leaveConversation  func(actor, channelID uuid.UUID) error
	listPublicChannels func(query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
	addMember          func(actor, conversationID, invitee uuid.UUID) (*domainconv.Member, error)
	removeMember       func(actor, conversationID, target uuid.UUID) error
	setMemberTier      func(actor, conversationID, target uuid.UUID, tier domainconv.AdminTier) (*domainconv.Member, error)
	listMembers        func(actor, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.MemberProfile, error)
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
func (f *fakeConversationService) LeaveConversation(_ context.Context, actor, channelID uuid.UUID) error {
	return f.leaveConversation(actor, channelID)
}
func (f *fakeConversationService) ListPublicChannels(_ context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error) {
	return f.listPublicChannels(query, before, beforeID, limit)
}
func (f *fakeConversationService) AddMember(_ context.Context, actor, conversationID, invitee uuid.UUID) (*domainconv.Member, error) {
	return f.addMember(actor, conversationID, invitee)
}
func (f *fakeConversationService) RemoveMember(_ context.Context, actor, conversationID, target uuid.UUID) error {
	return f.removeMember(actor, conversationID, target)
}
func (f *fakeConversationService) SetMemberTier(_ context.Context, actor, conversationID, target uuid.UUID, tier domainconv.AdminTier) (*domainconv.Member, error) {
	return f.setMemberTier(actor, conversationID, target, tier)
}
func (f *fakeConversationService) ListMembers(_ context.Context, actor, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.MemberProfile, error) {
	return f.listMembers(actor, conversationID, adminsOnly, before, beforeUserID, limit)
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

func TestConversationHandler_LeaveConversation(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	chID := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		leaveConversation: func(actor, channelID uuid.UUID) error {
			if actor != me || channelID != chID {
				t.Errorf("LeaveConversation(%v, %v)", actor, channelID)
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

func TestConversationHandler_LeaveConversation_NotMember(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &ConversationHandler{svc: &fakeConversationService{
		leaveConversation: func(uuid.UUID, uuid.UUID) error { return domainconv.ErrNotMember },
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

func TestConversationHandler_AddMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	invitee := uuid.Must(uuid.NewV7())
	var gotInvitee uuid.UUID
	svc := &fakeConversationService{addMember: func(_, _, inv uuid.UUID) (*domainconv.Member, error) {
		gotInvitee = inv
		return &domainconv.Member{UserID: inv, Role: domainconv.RoleMember, AdminTier: domainconv.TierNone}, nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)

	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/members", `{"user_id":"`+invitee.String()+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	if gotInvitee != invitee {
		t.Fatalf("invitee passed = %s", gotInvitee)
	}
}

func TestConversationHandler_AddMember_Forbidden(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	svc := &fakeConversationService{addMember: func(_, _, _ uuid.UUID) (*domainconv.Member, error) {
		return nil, usecaseconv.ErrForbidden
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/members", `{"user_id":"`+uuid.Must(uuid.NewV7()).String()+`"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestConversationHandler_RemoveMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	called := false
	svc := &fakeConversationService{removeMember: func(_, _, tg uuid.UUID) error {
		called = tg == target
		return nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodDelete, "/conversations/"+convID.String()+"/members/"+target.String(), "")
	if w.Code != http.StatusNoContent || !called {
		t.Fatalf("status = %d, called = %v", w.Code, called)
	}
}

func TestConversationHandler_RemoveMember_SelfIs400(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	svc := &fakeConversationService{removeMember: func(_, _, _ uuid.UUID) error { return usecaseconv.ErrCannotKickSelf }}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodDelete, "/conversations/"+convID.String()+"/members/"+actor.String(), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestConversationHandler_SetMemberTier(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	var gotTier domainconv.AdminTier
	svc := &fakeConversationService{setMemberTier: func(_, _, _ uuid.UUID, tr domainconv.AdminTier) (*domainconv.Member, error) {
		gotTier = tr
		return &domainconv.Member{UserID: target, Role: domainconv.RoleMember, AdminTier: tr, IsCreator: true}, nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/members/"+target.String(), `{"admin_tier":"admin"}`)
	if w.Code != http.StatusOK || gotTier != domainconv.TierAdmin {
		t.Fatalf("status = %d, tier = %q, body %s", w.Code, gotTier, w.Body)
	}
	// The response body is the only check that memberResponse's json tags and
	// toMemberResponse's mapping are correct.
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %s: %v", w.Body, err)
	}
	for _, key := range []string{"user_id", "admin_tier", "is_creator"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response body missing %q: %s", key, w.Body)
		}
	}
	var resp struct {
		UserID    string `json:"user_id"`
		AdminTier string `json:"admin_tier"`
		IsCreator bool   `json:"is_creator"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal typed body: %v", err)
	}
	if resp.UserID != target.String() || resp.AdminTier != string(domainconv.TierAdmin) || !resp.IsCreator {
		t.Fatalf("response = %+v, want user_id=%s admin_tier=admin is_creator=true", resp, target)
	}
}

func TestConversationHandler_SetMemberTier_ClearWithEmptyString(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	var gotTier domainconv.AdminTier = "sentinel"
	svc := &fakeConversationService{setMemberTier: func(_, _, _ uuid.UUID, tr domainconv.AdminTier) (*domainconv.Member, error) {
		gotTier = tr
		return &domainconv.Member{UserID: target}, nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/members/"+target.String(), `{"admin_tier":""}`)
	if w.Code != http.StatusOK || gotTier != domainconv.TierNone {
		t.Fatalf("status = %d, tier = %q", w.Code, gotTier)
	}
}

func TestConversationHandler_CreatorSelfTier(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	var gotTarget uuid.UUID
	svc := &fakeConversationService{setMemberTier: func(_, _, tg uuid.UUID, tr domainconv.AdminTier) (*domainconv.Member, error) {
		gotTarget = tg
		return &domainconv.Member{UserID: tg, AdminTier: tr, IsCreator: true}, nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/members/me", `{"admin_tier":"creator_admin"}`)
	if w.Code != http.StatusOK || gotTarget != actor {
		t.Fatalf("status = %d, target = %s", w.Code, gotTarget)
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
		listPublicChannels: func(string, time.Time, uuid.UUID, int) ([]*domainconv.Conversation, error) {
			return nil, nil
		},
		leaveConversation: func(uuid.UUID, uuid.UUID) error { return nil },
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
		{http.MethodGet, "/conversations/channels", ""},
		{http.MethodDelete, "/conversations/" + cid.String() + "/members/me", ""},
	} {
		w := doConvJSON(t, rt, tc.method, tc.path, tc.body)
		if w.Code == http.StatusNotFound {
			t.Fatalf("%s %s → 404 (route did not resolve)", tc.method, tc.path)
		}
	}
}

func TestConversationHandler_ListMembers(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	svc := &fakeConversationService{listMembers: func(_, _ uuid.UUID, adminsOnly bool, _ time.Time, _ uuid.UUID, _ int) ([]*domainconv.MemberProfile, error) {
		if !adminsOnly {
			t.Errorf("expected admins_only=true")
		}
		return []*domainconv.MemberProfile{{
			Member:   domainconv.Member{UserID: actor, Role: domainconv.RoleMember, AdminTier: domainconv.TierAdmin, JoinedAt: time.Now()},
			Username: "alice", DisplayName: "Alice", AvatarURL: "http://x/a.png",
		}}, nil
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/members?admins_only=true", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	var body struct {
		Members []struct {
			UserID    string `json:"user_id"`
			Username  string `json:"username"`
			AdminTier string `json:"admin_tier"`
		} `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Members) != 1 || body.Members[0].Username != "alice" || body.Members[0].AdminTier != "admin" {
		t.Fatalf("members = %+v", body.Members)
	}
}

func TestConversationHandler_ListMembers_NotMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	svc := &fakeConversationService{listMembers: func(_, _ uuid.UUID, _ bool, _ time.Time, _ uuid.UUID, _ int) ([]*domainconv.MemberProfile, error) {
		return nil, domainconv.ErrNotMember
	}}
	h := &ConversationHandler{svc: svc}
	r := convRouter(h, &actor)
	w := doConvJSON(t, r, http.MethodGet, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/members", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}
