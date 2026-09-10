package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	usecasecall "pet-messenger/internal/usecase/call"
)

type fakeCallService struct {
	start   func(actorID, conversationID uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error)
	join    func(actorID, callID uuid.UUID, role domaincall.Role) (usecasecall.JoinInfo, error)
	end     func(actorID, callID uuid.UUID) error
	mute    func(actorID, callID, targetID uuid.UUID) error
	remove  func(actorID, callID, targetID uuid.UUID) error
	getLive func(actorID, conversationID uuid.UUID) (*domaincall.Call, error)
}

func (f *fakeCallService) StartCall(_ context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error) {
	return f.start(actorID, conversationID)
}
func (f *fakeCallService) JoinCall(_ context.Context, actorID, callID uuid.UUID, role domaincall.Role) (usecasecall.JoinInfo, error) {
	return f.join(actorID, callID, role)
}
func (f *fakeCallService) EndCall(_ context.Context, actorID, callID uuid.UUID) error {
	return f.end(actorID, callID)
}
func (f *fakeCallService) MuteParticipant(_ context.Context, actorID, callID, targetID uuid.UUID) error {
	return f.mute(actorID, callID, targetID)
}
func (f *fakeCallService) RemoveParticipant(_ context.Context, actorID, callID, targetID uuid.UUID) error {
	return f.remove(actorID, callID, targetID)
}
func (f *fakeCallService) GetLiveForConversation(_ context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, error) {
	return f.getLive(actorID, conversationID)
}

func callRouter(svc callService, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &CallHandler{svc: svc}
	inject := func(c *gin.Context) {
		if actor != nil {
			c.Set(ContextUserIDKey, *actor)
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

func sampleCall(convID, initiatorID uuid.UUID) *domaincall.Call {
	return &domaincall.Call{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: convID,
		InitiatorID:    initiatorID,
		Status:         domaincall.StatusActive,
		MediaNodeID:    "node-a",
		StartedAt:      time.Now().UTC(),
	}
}

func TestCallHandler_Start_Created(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	call := sampleCall(convID, me)
	svc := &fakeCallService{
		start: func(actorID, conversationID uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error) {
			if actorID != me || conversationID != convID {
				t.Errorf("StartCall(%v,%v), want (%v,%v)", actorID, conversationID, me, convID)
			}
			return call, usecasecall.JoinInfo{Ticket: "tkt", MediaWSURL: "ws://x/rtc", Role: domaincall.RolePublisher}, nil
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
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
	svc := &fakeCallService{
		start: func(uuid.UUID, uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error) {
			return nil, usecasecall.JoinInfo{}, usecasecall.ErrCallNotSupported
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestCallHandler_Start_NoMediaNodes_ServiceUnavailable(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		start: func(uuid.UUID, uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error) {
			return nil, usecasecall.JoinInfo{}, usecasecall.ErrNoMediaNodes
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCallHandler_Start_BadConversationID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		start: func(uuid.UUID, uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error) {
			t.Fatal("service must not be called on a bad id")
			return nil, usecasecall.JoinInfo{}, nil
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/conversations/not-a-uuid/calls", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCallHandler_Join_RolePassedThrough(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	var gotRole domaincall.Role
	svc := &fakeCallService{
		join: func(actorID, gotCallID uuid.UUID, role domaincall.Role) (usecasecall.JoinInfo, error) {
			gotRole = role
			if actorID != me || gotCallID != callID {
				t.Errorf("JoinCall(%v,%v), want (%v,%v)", actorID, gotCallID, me, callID)
			}
			return usecasecall.JoinInfo{Ticket: "tkt", Role: role}, nil
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/join", `{"role":"viewer"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if gotRole != domaincall.RoleViewer {
		t.Fatalf("role = %q, want viewer", gotRole)
	}
}

func TestCallHandler_Join_BadCallID(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		join: func(uuid.UUID, uuid.UUID, domaincall.Role) (usecasecall.JoinInfo, error) {
			t.Fatal("service must not be called on a bad id")
			return usecasecall.JoinInfo{}, nil
		},
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/calls/nope/join", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCallHandler_End_AlreadyEnded_NoContent(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		end: func(uuid.UUID, uuid.UUID) error { return domaincall.ErrAlreadyEnded },
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/end", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestCallHandler_Remove_Forbidden(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		remove: func(uuid.UUID, uuid.UUID, uuid.UUID) error { return usecasecall.ErrForbidden },
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodDelete, "/calls/"+callID.String()+"/participants/"+target.String(), "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestCallHandler_Live_NotFound(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		getLive: func(uuid.UUID, uuid.UUID) (*domaincall.Call, error) { return nil, domaincall.ErrNotFound },
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCallHandler_Mute_ConversationArchived_Conflict(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())
	svc := &fakeCallService{
		mute: func(uuid.UUID, uuid.UUID, uuid.UUID) error { return conversation.ErrConversationArchived },
	}
	r := callRouter(svc, &me)
	w := doConvJSON(t, r, http.MethodPost, "/calls/"+callID.String()+"/participants/"+target.String()+"/mute", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}
