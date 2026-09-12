package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/user"
)

type fakeUserLookupRepository struct {
	user *user.User
	err  error
}

func (f *fakeUserLookupRepository) GetByUsername(context.Context, string) (*user.User, error) {
	return f.user, f.err
}

func userRouter(h *UserHandler) *gin.Engine {
	r := gin.New()
	h.RegisterRoutes(r.Group("/users"))
	return r
}

func TestUserHandler_GetByUsername(t *testing.T) {
	u := &user.User{ID: uuid.Must(uuid.NewV7()), Username: "bob_01", DisplayName: "Bob"}
	w := httptest.NewRecorder()
	userRouter(&UserHandler{users: &fakeUserLookupRepository{user: u}}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/bob_01", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var response userProfileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.ID != u.ID.String() || response.Username != u.Username {
		t.Errorf("profile = %#v, want id=%q username=%q", response, u.ID, u.Username)
	}
}

func TestUserHandler_GetByUsername_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	userRouter(&UserHandler{users: &fakeUserLookupRepository{err: user.ErrNotFound}}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUserHandler_GetByUsername_InternalError(t *testing.T) {
	w := httptest.NewRecorder()
	userRouter(&UserHandler{users: &fakeUserLookupRepository{err: errors.New("database unavailable")}}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/bob_01", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
