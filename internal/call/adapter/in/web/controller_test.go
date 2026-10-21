package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	callcommand "pet-messenger/internal/call/application/command"
	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
)

type fakeStartCall struct {
	fn func(cmd in.StartCallCommand) (*domain.Call, out.JoinInfo, error)
}

func (f *fakeStartCall) Handle(_ context.Context, cmd in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
	return f.fn(cmd)
}

type fakeJoinCall struct {
	fn func(cmd in.JoinCallCommand) (out.JoinInfo, error)
}

func (f *fakeJoinCall) Handle(_ context.Context, cmd in.JoinCallCommand) (out.JoinInfo, error) {
	return f.fn(cmd)
}

type fakeEndCall struct {
	fn func(cmd in.EndCallCommand) error
}

func (f *fakeEndCall) Handle(_ context.Context, cmd in.EndCallCommand) error { return f.fn(cmd) }

type fakeMuteParticipant struct {
	fn func(cmd in.MuteParticipantCommand) error
}

func (f *fakeMuteParticipant) Handle(_ context.Context, cmd in.MuteParticipantCommand) error {
	return f.fn(cmd)
}

type fakeUnmuteParticipant struct {
	fn func(cmd in.UnmuteParticipantCommand) error
}

func (f *fakeUnmuteParticipant) Handle(_ context.Context, cmd in.UnmuteParticipantCommand) error {
	return f.fn(cmd)
}

type fakeRemoveParticipant struct {
	fn func(cmd in.RemoveParticipantCommand) error
}

func (f *fakeRemoveParticipant) Handle(_ context.Context, cmd in.RemoveParticipantCommand) error {
	return f.fn(cmd)
}

type fakeGetLiveForConversation struct {
	fn func(q in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error)
}

func (f *fakeGetLiveForConversation) Handle(_ context.Context, q in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error) {
	return f.fn(q)
}

func callRouter(h *CallHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	inject := func(c *gin.Context) {
		if actor != nil {
			c.Set(httpadapter.ContextUserIDKey, *actor)
		}
		c.Next()
	}
	convs := r.Group("/conversations")
	convs.Use(inject)
	h.RegisterConversationRoutes(convs)
	calls := r.Group("/calls")
	calls.Use(inject)
	h.RegisterRoutes(calls)
	return r
}

func doCallJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sampleCall(convID, initiatorID uuid.UUID) *domain.Call {
	return &domain.Call{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: convID,
		InitiatorID:    initiatorID,
		Status:         domain.StatusActive,
		MediaNodeID:    "node-a",
		StartedAt:      time.Now().UTC(),
	}
}

func TestCallHandler_Start_Created(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	call := sampleCall(convID, me)
	start := &fakeStartCall{fn: func(cmd in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
		if cmd.Actor != me || cmd.ConversationID != convID {
			t.Errorf("cmd = %+v, want actor=%v conv=%v", cmd, me, convID)
		}
		return call, out.JoinInfo{Ticket: "tkt", MediaWSURL: "ws://x/rtc", Role: domain.RolePublisher}, nil
	}}
	h := NewCallHandler(start, nil, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Call struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"call"`
		Join struct {
			Ticket string `json:"ticket"`
			Role   string `json:"role"`
		} `json:"join"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Call.ID != call.ID.String() || body.Call.Status != "active" {
		t.Errorf("call = %+v", body.Call)
	}
	if body.Join.Ticket != "tkt" || body.Join.Role != "publisher" {
		t.Errorf("join = %+v", body.Join)
	}
}

func TestCallHandler_Start_NotSupported_Conflict(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	start := &fakeStartCall{fn: func(in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
		return nil, out.JoinInfo{}, callcommand.ErrCallNotSupported
	}}
	h := NewCallHandler(start, nil, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestCallHandler_Start_NoMediaNodes_ServiceUnavailable(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	start := &fakeStartCall{fn: func(in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
		return nil, out.JoinInfo{}, callcommand.ErrNoMediaNodes
	}}
	h := NewCallHandler(start, nil, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCallHandler_Start_BadConversationID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	start := &fakeStartCall{fn: func(in.StartCallCommand) (*domain.Call, out.JoinInfo, error) {
		t.Fatal("use case must not be called on a bad id")
		return nil, out.JoinInfo{}, nil
	}}
	h := NewCallHandler(start, nil, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/conversations/not-a-uuid/calls", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCallHandler_Join_RolePassedThrough(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	var gotRole domain.Role
	join := &fakeJoinCall{fn: func(cmd in.JoinCallCommand) (out.JoinInfo, error) {
		gotRole = cmd.Role
		if cmd.Actor != me || cmd.CallID != callID {
			t.Errorf("cmd = %+v, want actor=%v call=%v", cmd, me, callID)
		}
		return out.JoinInfo{Ticket: "tkt", Role: cmd.Role}, nil
	}}
	h := NewCallHandler(nil, join, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/join", `{"role":"viewer"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if gotRole != domain.RoleViewer {
		t.Fatalf("role = %q, want viewer", gotRole)
	}
}

func TestCallHandler_Join_BadCallID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	join := &fakeJoinCall{fn: func(in.JoinCallCommand) (out.JoinInfo, error) {
		t.Fatal("use case must not be called on a bad id")
		return out.JoinInfo{}, nil
	}}
	h := NewCallHandler(nil, join, nil, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/calls/nope/join", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCallHandler_End_AlreadyEnded_NoContent(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	end := &fakeEndCall{fn: func(in.EndCallCommand) error { return domain.ErrAlreadyEnded }}
	h := NewCallHandler(nil, nil, end, nil, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/end", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestCallHandler_Remove_Forbidden(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	remove := &fakeRemoveParticipant{fn: func(in.RemoveParticipantCommand) error { return callcommand.ErrForbidden }}
	h := NewCallHandler(nil, nil, nil, nil, nil, remove, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodDelete, "/calls/"+callID.String()+"/participants/"+target.String(), "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestCallHandler_Live_NotFound(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	live := &fakeGetLiveForConversation{fn: func(in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error) {
		return nil, nil, domain.ErrNotFound
	}}
	h := NewCallHandler(nil, nil, nil, nil, nil, nil, live)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCallHandler_Live_IncludesParticipants(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	call := sampleCall(convID, me)
	participants := []out.ParticipantInfo{
		{UserID: me.String(), Role: "publisher", Muted: false, SelfMuted: true, TrackKinds: []string{"mic", "camera"}},
	}
	live := &fakeGetLiveForConversation{fn: func(q in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error) {
		if q.Actor != me || q.ConversationID != convID {
			t.Errorf("q = %+v, want actor=%v conv=%v", q, me, convID)
		}
		return call, participants, nil
	}}
	h := NewCallHandler(nil, nil, nil, nil, nil, nil, live)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Call struct {
			ID string `json:"id"`
		} `json:"call"`
		Participants []struct {
			UserID     string   `json:"user_id"`
			Role       string   `json:"role"`
			Muted      bool     `json:"muted"`
			SelfMuted  bool     `json:"self_muted"`
			TrackKinds []string `json:"track_kinds"`
		} `json:"participants"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Call.ID != call.ID.String() {
		t.Errorf("call.id = %q, want %q", body.Call.ID, call.ID.String())
	}
	if len(body.Participants) != 1 {
		t.Fatalf("participants = %+v, want 1 entry", body.Participants)
	}
	got := body.Participants[0]
	if got.UserID != me.String() || got.Role != "publisher" || got.Muted || !got.SelfMuted {
		t.Errorf("participant = %+v", got)
	}
	if len(got.TrackKinds) != 2 || got.TrackKinds[0] != "mic" || got.TrackKinds[1] != "camera" {
		t.Errorf("track_kinds = %v", got.TrackKinds)
	}
}

func TestCallHandler_Live_EmptyParticipantsNotNull(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	call := sampleCall(convID, me)
	live := &fakeGetLiveForConversation{fn: func(in.GetLiveForConversationQuery) (*domain.Call, []out.ParticipantInfo, error) {
		return call, []out.ParticipantInfo{}, nil
	}}
	h := NewCallHandler(nil, nil, nil, nil, nil, nil, live)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw, ok := body["participants"]
	if !ok {
		t.Fatal("participants key missing from response")
	}
	if string(raw) != "[]" {
		t.Errorf("participants = %s, want []", raw)
	}
}

func TestCallHandler_Mute_ConversationArchived_Conflict(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	mute := &fakeMuteParticipant{fn: func(in.MuteParticipantCommand) error { return conversation.ErrConversationArchived }}
	h := NewCallHandler(nil, nil, nil, mute, nil, nil, nil)
	r := callRouter(h, &me)
	w := doCallJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/participants/"+target.String()+"/mute", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestCallHandler_Unmute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"success", nil, http.StatusNoContent},
		{"forbidden", callcommand.ErrForbidden, http.StatusForbidden},
		{"self", callcommand.ErrCannotTargetSelf, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor, callID, target := uuid.New(), uuid.New(), uuid.New()
			called := false
			unmute := &fakeUnmuteParticipant{fn: func(cmd in.UnmuteParticipantCommand) error {
				called = true
				if cmd.Actor != actor || cmd.CallID != callID || cmd.TargetID != target {
					t.Fatal("incorrect unmute IDs")
				}
				return tc.err
			}}
			h := NewCallHandler(nil, nil, nil, nil, unmute, nil, nil)
			w := doCallJSON(t, callRouter(h, &actor), http.MethodPost, "/calls/"+callID.String()+"/participants/"+target.String()+"/unmute", "")
			if !called || w.Code != tc.status {
				t.Fatalf("called=%v status=%d, want %d", called, w.Code, tc.status)
			}
		})
	}
}
