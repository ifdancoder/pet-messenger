package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/user"
	"pet-messenger/internal/infrastructure/objectstore"
)

// fakeUserLookupRepository implements user.Repository but only meaningfully
// serves GetByUsername, with an injectable error — the pre-existing tests
// below exercise GetByUsername's response mapping and predate the
// profile-update/avatar endpoints added in this file.
type fakeUserLookupRepository struct {
	user *user.User
	err  error
}

func (f *fakeUserLookupRepository) Create(context.Context, *user.User) error { return f.err }
func (f *fakeUserLookupRepository) GetByID(context.Context, uuid.UUID) (*user.User, error) {
	return f.user, f.err
}
func (f *fakeUserLookupRepository) GetByUsername(context.Context, string) (*user.User, error) {
	return f.user, f.err
}
func (f *fakeUserLookupRepository) Update(context.Context, *user.User) error { return f.err }

type fakeUserRepoForHandler struct {
	byID map[uuid.UUID]*user.User
}

func (f *fakeUserRepoForHandler) Create(context.Context, *user.User) error { return nil }
func (f *fakeUserRepoForHandler) GetByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (f *fakeUserRepoForHandler) GetByUsername(_ context.Context, username string) (*user.User, error) {
	for _, u := range f.byID {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, user.ErrNotFound
}
func (f *fakeUserRepoForHandler) Update(_ context.Context, u *user.User) error {
	f.byID[u.ID] = u
	return nil
}

type fakeAvatarStore struct {
	data        map[uuid.UUID][]byte
	contentType map[uuid.UUID]string
}

func (f *fakeAvatarStore) Put(_ context.Context, userID uuid.UUID, contentType string, data io.Reader, _ int64) error {
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	if f.data == nil {
		f.data = map[uuid.UUID][]byte{}
		f.contentType = map[uuid.UUID]string{}
	}
	f.data[userID] = b
	f.contentType[userID] = contentType
	return nil
}
func (f *fakeAvatarStore) Get(_ context.Context, userID uuid.UUID) (io.ReadCloser, string, error) {
	b, ok := f.data[userID]
	if !ok {
		return nil, "", objectstore.ErrAvatarNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), f.contentType[userID], nil
}

func userRouter(h *UserHandler, actor uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rg := r.Group("/users")
	rg.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, actor); c.Next() })
	h.RegisterRoutes(rg)
	return r
}

func TestUserHandler_UpdateProfile(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	u, _ := user.New("alice", "Alice")
	u.ID = me
	repo := &fakeUserRepoForHandler{byID: map[uuid.UUID]*user.User{me: u}}
	h := NewUserHandler(repo, &fakeAvatarStore{})
	r := userRouter(h, me)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewBufferString(`{"display_name":"Alice B"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.byID[me].DisplayName != "Alice B" {
		t.Fatalf("DisplayName = %q, want Alice B", repo.byID[me].DisplayName)
	}
}

func TestUserHandler_UploadAndServeAvatar(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	u, _ := user.New("alice", "Alice")
	u.ID = me
	repo := &fakeUserRepoForHandler{byID: map[uuid.UUID]*user.User{me: u}}
	store := &fakeAvatarStore{}
	h := NewUserHandler(repo, store)
	r := userRouter(h, me)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("avatar", "avatar.png")
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	_, _ = part.Write(append(pngHeader, bytes.Repeat([]byte{0}, 100)...))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.byID[me].AvatarURL == "" {
		t.Fatalf("AvatarURL not set after upload")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/users/alice/avatar", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("serve status = %d", rec2.Code)
	}
	if rec2.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", rec2.Header().Get("Content-Type"))
	}
}

func TestUserHandler_UpdateProfile_UsernameCollision(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	u, _ := user.New("alice", "Alice")
	u.ID = me
	repo := &collidingUserRepo{fakeUserRepoForHandler: fakeUserRepoForHandler{byID: map[uuid.UUID]*user.User{me: u}}}
	h := NewUserHandler(repo, &fakeAvatarStore{})
	r := userRouter(h, me)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewBufferString(`{"username":"taken"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

type collidingUserRepo struct{ fakeUserRepoForHandler }

func (c *collidingUserRepo) Update(context.Context, *user.User) error { return user.ErrUsernameTaken }

func TestUserHandler_GetByUsername(t *testing.T) {
	u := &user.User{ID: uuid.Must(uuid.NewV7()), Username: "bob_01", DisplayName: "Bob"}
	h := NewUserHandler(&fakeUserLookupRepository{user: u}, &fakeAvatarStore{})
	w := httptest.NewRecorder()
	userRouter(h, uuid.Nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/bob_01", nil))

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
	h := NewUserHandler(&fakeUserLookupRepository{err: user.ErrNotFound}, &fakeAvatarStore{})
	w := httptest.NewRecorder()
	userRouter(h, uuid.Nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUserHandler_GetByUsername_InternalError(t *testing.T) {
	h := NewUserHandler(&fakeUserLookupRepository{err: errors.New("database unavailable")}, &fakeAvatarStore{})
	w := httptest.NewRecorder()
	userRouter(h, uuid.Nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/bob_01", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
