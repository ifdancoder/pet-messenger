// internal/adapter/http/auth_handler_test.go
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	usecaseauth "pet-messenger/internal/usecase/auth"
	domainuser "pet-messenger/internal/user/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeAuthService struct {
	result *usecaseauth.Result
	err    error
}

func (f *fakeAuthService) Register(context.Context, string, string, string) (*usecaseauth.Result, error) {
	return f.result, f.err
}
func (f *fakeAuthService) Login(context.Context, string, string, string) (*usecaseauth.Result, error) {
	return f.result, f.err
}
func (f *fakeAuthService) Refresh(context.Context, string) (*usecaseauth.Result, error) {
	return f.result, f.err
}
func (f *fakeAuthService) Logout(context.Context, string) error {
	return f.err
}

func newTestResult() *usecaseauth.Result {
	u, _ := domainuser.New("alice_01", "Alice")
	return &usecaseauth.Result{
		User:            u,
		AccessToken:     "access-token",
		AccessExpiresAt: time.Now().Add(15 * time.Minute),
		RefreshToken:    "refresh-token",
	}
}

func setupRouter(h *AuthHandler) *gin.Engine {
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
	h := &AuthHandler{svc: &fakeAuthService{result: newTestResult()}}
	r := setupRouter(h)

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
	h := &AuthHandler{svc: &fakeAuthService{result: newTestResult()}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthHandler_Register_UsernameTaken(t *testing.T) {
	h := &AuthHandler{svc: &fakeAuthService{err: domainuser.ErrUsernameTaken}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{
		"username": "alice_01",
		"password": "password123",
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	h := &AuthHandler{svc: &fakeAuthService{err: usecaseauth.ErrInvalidCredentials}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/login", map[string]string{
		"username": "alice_01",
		"password": "wrong",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Refresh(t *testing.T) {
	h := &AuthHandler{svc: &fakeAuthService{result: newTestResult()}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": "some-token"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAuthHandler_Refresh_Invalid(t *testing.T) {
	h := &AuthHandler{svc: &fakeAuthService{err: usecaseauth.ErrInvalidRefreshToken}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{"refresh_token": "bad"})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	h := &AuthHandler{svc: &fakeAuthService{}}
	r := setupRouter(h)

	w := doJSON(t, r, http.MethodPost, "/auth/logout", map[string]string{"refresh_token": "some-token"})

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
