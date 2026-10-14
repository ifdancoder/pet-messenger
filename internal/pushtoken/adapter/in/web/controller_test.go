// internal/pushtoken/adapter/in/web/controller_test.go
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

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/pushtoken/application/port/in"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

type fakeRegister struct {
	handle func(cmd in.RegisterPushTokenCommand) (*domainpush.PushToken, error)
}

func (f *fakeRegister) Handle(_ context.Context, cmd in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
	return f.handle(cmd)
}

type fakeUnregister struct {
	handle func(cmd in.UnregisterPushTokenCommand) error
}

func (f *fakeUnregister) Handle(_ context.Context, cmd in.UnregisterPushTokenCommand) error {
	return f.handle(cmd)
}

func pushTokRouter(h *PushTokenHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/push-tokens")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(httpadapter.ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func doPush(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPushTokenHandler_Register_Created(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	var gotCmd in.RegisterPushTokenCommand
	h := NewPushTokenHandler(&fakeRegister{
		handle: func(cmd in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
			gotCmd = cmd
			return &domainpush.PushToken{
				ID: uuid.Must(uuid.NewV7()), UserID: cmd.Actor, Platform: cmd.Platform,
				Token: cmd.Token, CreatedAt: now, LastUsedAt: now,
			}, nil
		},
	}, &fakeUnregister{})
	r := pushTokRouter(h, &me)
	w := doPush(t, r, http.MethodPost, "/push-tokens", `{"platform":"web","token":"endpoint-abc"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if gotCmd.Actor != me || gotCmd.Platform != domainpush.PlatformWeb || gotCmd.Token != "endpoint-abc" {
		t.Errorf("Handle got %+v", gotCmd)
	}
	var body struct {
		ID         string    `json:"id"`
		Platform   string    `json:"platform"`
		Token      string    `json:"token"`
		CreatedAt  time.Time `json:"created_at"`
		LastUsedAt time.Time `json:"last_used_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Platform != "web" || body.Token != "endpoint-abc" || body.ID == "" {
		t.Errorf("body = %+v", body)
	}
	if body.CreatedAt.IsZero() || body.LastUsedAt.IsZero() {
		t.Errorf("timestamps not serialised: %+v", body)
	}
}

func TestPushTokenHandler_Register_MissingFields(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewPushTokenHandler(&fakeRegister{
		handle: func(in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
			t.Fatal("Handle must not be called on a bad request body")
			return nil, nil
		},
	}, &fakeUnregister{})
	r := pushTokRouter(h, &me)
	w := doPush(t, r, http.MethodPost, "/push-tokens", `{"platform":"web"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPushTokenHandler_Register_InvalidPlatform(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewPushTokenHandler(&fakeRegister{
		handle: func(in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
			return nil, domainpush.ErrInvalidPlatform
		},
	}, &fakeUnregister{})
	r := pushTokRouter(h, &me)
	w := doPush(t, r, http.MethodPost, "/push-tokens", `{"platform":"carrier-pigeon","token":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPushTokenHandler_Register_Unauthenticated(t *testing.T) {
	h := NewPushTokenHandler(&fakeRegister{
		handle: func(in.RegisterPushTokenCommand) (*domainpush.PushToken, error) {
			t.Fatal("Handle must not be called without an actor")
			return nil, nil
		},
	}, &fakeUnregister{})
	r := pushTokRouter(h, nil)
	w := doPush(t, r, http.MethodPost, "/push-tokens", `{"platform":"web","token":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestPushTokenHandler_Unregister_NoContent(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	var gotCmd in.UnregisterPushTokenCommand
	h := NewPushTokenHandler(&fakeRegister{}, &fakeUnregister{
		handle: func(cmd in.UnregisterPushTokenCommand) error {
			gotCmd = cmd
			return nil
		},
	})
	r := pushTokRouter(h, &me)
	w := doPush(t, r, http.MethodDelete, "/push-tokens", `{"token":"endpoint-abc"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if gotCmd.Actor != me || gotCmd.Token != "endpoint-abc" {
		t.Errorf("Handle got %+v", gotCmd)
	}
}

func TestPushTokenHandler_Unregister_MissingToken(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewPushTokenHandler(&fakeRegister{}, &fakeUnregister{
		handle: func(in.UnregisterPushTokenCommand) error {
			t.Fatal("Handle must not be called without a token")
			return nil
		},
	})
	r := pushTokRouter(h, &me)
	w := doPush(t, r, http.MethodDelete, "/push-tokens", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
