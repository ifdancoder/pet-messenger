// internal/auth/adapter/in/web/controller_test.go
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

	"pet-messenger/internal/auth/application/command"
	"pet-messenger/internal/auth/application/port/in"
	domainuser "pet-messenger/internal/user/domain"
)

type fakeRegister struct {
	result *in.Result
	err    error
}

func (f *fakeRegister) Handle(context.Context, in.RegisterCommand) (*in.Result, error) {
	return f.result, f.err
}

type fakeLogin struct {
	result *in.Result
	err    error
}

func (f *fakeLogin) Handle(context.Context, in.LoginCommand) (*in.Result, error) {
	return f.result, f.err
}

type fakeRefresh struct {
	result *in.Result
	err    error
}

func (f *fakeRefresh) Handle(context.Context, in.RefreshCommand) (*in.Result, error) {
	return f.result, f.err
}

type fakeLogout struct {
	err error
}

func (f *fakeLogout) Handle(context.Context, in.LogoutCommand) error {
	return f.err
}

func newTestResult(t *testing.T) *in.Result {
	t.Helper()
	u, err := domainuser.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	return &in.Result{
		User:            u,
		AccessToken:     "access-token",
		AccessExpiresAt: time.Now().Add(15 * time.Minute),
		RefreshToken:    "refresh-token",
	}
}

func newTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r.Group("/auth"))
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthHandler_Register(t *testing.T) {
	h := &AuthHandler{register: &fakeRegister{result: newTestResult(t)}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{
		"username": "alice_01",
		"password": "password123",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "access-token")
	}
	if resp.User.Username != "alice_01" {
		t.Errorf("User.Username = %q, want %q", resp.User.Username, "alice_01")
	}
}

func TestAuthHandler_Register_MissingFields(t *testing.T) {
	h := &AuthHandler{register: &fakeRegister{result: newTestResult(t)}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthHandler_Register_UsernameTaken(t *testing.T) {
	h := &AuthHandler{register: &fakeRegister{err: domainuser.ErrUsernameTaken}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{
		"username": "alice_01",
		"password": "password123",
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	h := &AuthHandler{login: &fakeLogin{err: command.ErrInvalidCredentials}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/login", map[string]string{
		"username": "alice_01",
		"password": "wrong",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Refresh(t *testing.T) {
	h := &AuthHandler{refresh: &fakeRefresh{result: newTestResult(t)}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": "some-token"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAuthHandler_Refresh_Invalid(t *testing.T) {
	h := &AuthHandler{refresh: &fakeRefresh{err: command.ErrInvalidRefreshToken}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": "bad"})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	h := &AuthHandler{logout: &fakeLogout{}}
	r := newTestRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/logout", map[string]string{"refresh_token": "some-token"})

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
