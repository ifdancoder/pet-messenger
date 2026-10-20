package web

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

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/domain"
)

type fakeGetByUsername struct {
	user *domain.User
	err  error
}

func (f *fakeGetByUsername) Handle(context.Context, in.GetByUsernameQuery) (*domain.User, error) {
	return f.user, f.err
}

type fakeServeAvatar struct {
	data io.ReadCloser
	ct   string
	err  error
}

func (f *fakeServeAvatar) Handle(context.Context, in.ServeAvatarQuery) (io.ReadCloser, string, error) {
	return f.data, f.ct, f.err
}

type fakeUpdateProfile struct {
	gotCmd in.UpdateProfileCommand
	user   *domain.User
	err    error
}

func (f *fakeUpdateProfile) Handle(_ context.Context, cmd in.UpdateProfileCommand) (*domain.User, error) {
	f.gotCmd = cmd
	return f.user, f.err
}

type fakeUploadAvatar struct {
	user *domain.User
	err  error
}

func (f *fakeUploadAvatar) Handle(context.Context, in.UploadAvatarCommand) (*domain.User, error) {
	return f.user, f.err
}

// newTestRouter wires h behind a real gin engine so tests exercise the
// actual ServeHTTP dispatch path (status/header writing, route param
// parsing) instead of calling handler methods directly against a bare
// gin.Context: a direct call never triggers gin's deferred
// WriteHeaderNow, which would make a bare c.Status(...) response
// (ServeAvatar's 404 path) appear as a 200 in the test even though it is
// correct in production. When hasUser is true, userID is injected via a
// middleware exactly as httpadapter.AuthMiddleware would.
func newTestRouter(t *testing.T, h *UserHandler, userID uuid.UUID, hasUser bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rg := r.Group("/users")
	if hasUser {
		rg.Use(func(c *gin.Context) {
			c.Set(httpadapter.ContextUserIDKey, userID)
			c.Next()
		})
	}
	h.RegisterRoutes(rg)
	return r
}

func TestGetByUsername_Found(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := &UserHandler{getByUsername: &fakeGetByUsername{user: u}}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodGet, "/users/alice_01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp userProfileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Username != "alice_01" {
		t.Fatalf("Username = %q, want alice_01", resp.Username)
	}
}

func TestGetByUsername_NotFound(t *testing.T) {
	h := &UserHandler{getByUsername: &fakeGetByUsername{err: domain.ErrNotFound}}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodGet, "/users/nobody", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGetByUsername_InternalError(t *testing.T) {
	h := &UserHandler{getByUsername: &fakeGetByUsername{err: errors.New("boom")}}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodGet, "/users/alice_01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestUpdateProfile_Unauthenticated(t *testing.T) {
	h := &UserHandler{}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestUpdateProfile_NoFields(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	updated, err := domain.New("alice_02", "Alicia")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	fake := &fakeUpdateProfile{user: updated}
	h := &UserHandler{updateProfile: fake}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"display_name":"Alicia"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if fake.gotCmd.UserID != me {
		t.Fatalf("UserID = %v, want %v", fake.gotCmd.UserID, me)
	}
	if fake.gotCmd.DisplayName == nil || *fake.gotCmd.DisplayName != "Alicia" {
		t.Fatalf("DisplayName = %v, want Alicia", fake.gotCmd.DisplayName)
	}
}

// writeProfileError sends err.Error() as the response body. These three
// tests pin the exact body, not just the status, because a future edit
// that wraps one of these sentinels (e.g. fmt.Errorf("...: %w", err))
// would still satisfy errors.Is and keep the status correct while
// silently changing the client-visible error text.

func TestUpdateProfile_UsernameTakenMapsTo409(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{updateProfile: &fakeUpdateProfile{err: domain.ErrUsernameTaken}}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"username":"taken"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", w.Code, w.Body.String())
	}
	wantBody := `{"error":"` + domain.ErrUsernameTaken.Error() + `"}`
	if got := w.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestUpdateProfile_InvalidDisplayNameMapsTo400(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{updateProfile: &fakeUpdateProfile{err: domain.ErrInvalidDisplayName}}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"display_name":""}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
	wantBody := `{"error":"` + domain.ErrInvalidDisplayName.Error() + `"}`
	if got := w.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestUpdateProfile_InvalidUsernameMapsTo400(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{updateProfile: &fakeUpdateProfile{err: domain.ErrInvalidUsername}}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"username":"a"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
	wantBody := `{"error":"` + domain.ErrInvalidUsername.Error() + `"}`
	if got := w.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestUpdateProfile_UnrecognizedErrorMapsTo500(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{updateProfile: &fakeUpdateProfile{err: errors.New("boom")}}
	r := newTestRouter(t, h, me, true)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"display_name":"Alicia"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func multipartAvatarBody(t *testing.T, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("avatar", "avatar.bin")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// pngMagicBytes is the minimal valid PNG signature http.DetectContentType
// recognizes as image/png.
var pngMagicBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func TestUploadAvatar_Success(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := &UserHandler{uploadAvatar: &fakeUploadAvatar{user: u}}
	r := newTestRouter(t, h, me, true)
	body, ct := multipartAvatarBody(t, pngMagicBytes)

	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadAvatar_RejectsUnknownType(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{uploadAvatar: &fakeUploadAvatar{}}
	r := newTestRouter(t, h, me, true)
	body, ct := multipartAvatarBody(t, []byte("not an image"))

	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

type closingReader struct{ *bytes.Reader }

func (closingReader) Close() error { return nil }

func TestServeAvatar_Found(t *testing.T) {
	avatarBytes := []byte("png-bytes")
	h := &UserHandler{serveAvatar: &fakeServeAvatar{data: closingReader{bytes.NewReader(avatarBytes)}, ct: "image/png"}}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodGet, "/users/alice_01/avatar", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), avatarBytes) {
		t.Fatalf("body = %q, want %q", w.Body.Bytes(), avatarBytes)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q, want %q", cc, "private, max-age=300")
	}
	if xcto := w.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", xcto)
	}
}

func TestServeAvatar_NotFound(t *testing.T) {
	h := &UserHandler{serveAvatar: &fakeServeAvatar{err: errors.New("not found")}}
	r := newTestRouter(t, h, uuid.Nil, false)

	req := httptest.NewRequest(http.MethodGet, "/users/alice_01/avatar", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
