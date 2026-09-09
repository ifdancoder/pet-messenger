package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/pushtoken"
)

type fakePushTokenRepo struct {
	upsert      func(t *pushtoken.PushToken) (*pushtoken.PushToken, error)
	deleteByTok func(userID uuid.UUID, token string) error
}

func (f *fakePushTokenRepo) Upsert(_ context.Context, t *pushtoken.PushToken) (*pushtoken.PushToken, error) {
	return f.upsert(t)
}
func (f *fakePushTokenRepo) DeleteByToken(_ context.Context, userID uuid.UUID, token string) error {
	return f.deleteByTok(userID, token)
}
func (f *fakePushTokenRepo) DeleteToken(context.Context, string) error { return nil }
func (f *fakePushTokenRepo) ListForUser(context.Context, uuid.UUID) ([]*pushtoken.PushToken, error) {
	return nil, nil
}

func pushTokRouter(h *PushTokenHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/push-tokens")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func TestPushTokenHandler_Register_Created(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	var gotUserID uuid.UUID
	var gotPlatform pushtoken.Platform
	var gotToken string
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		upsert: func(tok *pushtoken.PushToken) (*pushtoken.PushToken, error) {
			gotUserID, gotPlatform, gotToken = tok.UserID, tok.Platform, tok.Token
			tok.CreatedAt, tok.LastUsedAt = now, now
			return tok, nil
		},
	}}
	r := pushTokRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/push-tokens", `{"platform":"web","token":"endpoint-abc"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if gotUserID != me || gotPlatform != pushtoken.PlatformWeb || gotToken != "endpoint-abc" {
		t.Errorf("repo got (user=%v platform=%q token=%q)", gotUserID, gotPlatform, gotToken)
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
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		upsert: func(*pushtoken.PushToken) (*pushtoken.PushToken, error) {
			t.Fatal("repo must not be called on a bad request body")
			return nil, nil
		},
	}}
	r := pushTokRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/push-tokens", `{"platform":"web"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPushTokenHandler_Register_BadPlatform(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		upsert: func(*pushtoken.PushToken) (*pushtoken.PushToken, error) {
			t.Fatal("repo must not be called for an invalid platform")
			return nil, nil
		},
	}}
	r := pushTokRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/push-tokens", `{"platform":"carrier-pigeon","token":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPushTokenHandler_Register_Unauthenticated(t *testing.T) {
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		upsert: func(*pushtoken.PushToken) (*pushtoken.PushToken, error) {
			t.Fatal("repo must not be called without an actor")
			return nil, nil
		},
	}}
	r := pushTokRouter(h, nil)
	w := doConvJSON(t, r, http.MethodPost, "/push-tokens", `{"platform":"web","token":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestPushTokenHandler_Unregister_NoContent(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	var gotUserID uuid.UUID
	var gotToken string
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		deleteByTok: func(userID uuid.UUID, token string) error {
			gotUserID, gotToken = userID, token
			return nil
		},
	}}
	r := pushTokRouter(h, &me)
	w := doConvJSON(t, r, http.MethodDelete, "/push-tokens", `{"token":"endpoint-abc"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if gotUserID != me || gotToken != "endpoint-abc" {
		t.Errorf("repo got (user=%v token=%q)", gotUserID, gotToken)
	}
}

func TestPushTokenHandler_Unregister_MissingToken(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &PushTokenHandler{repo: &fakePushTokenRepo{
		deleteByTok: func(uuid.UUID, string) error {
			t.Fatal("repo must not be called without a token")
			return nil
		},
	}}
	r := pushTokRouter(h, &me)
	w := doConvJSON(t, r, http.MethodDelete, "/push-tokens", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
