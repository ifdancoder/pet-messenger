// internal/user/adapter/in/web/controller_test.go
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

func newTestContext(method, path string, body io.Reader, userID uuid.UUID, hasUser bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, body)
	if hasUser {
		c.Set(httpadapter.ContextUserIDKey, userID)
	}
	return c, w
}

func TestGetByUsername_Found(t *testing.T) {
	u, err := domain.New("alice_01", "Alice")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	h := &UserHandler{getByUsername: &fakeGetByUsername{user: u}}
	c, w := newTestContext(http.MethodGet, "/users/alice_01", nil, uuid.Nil, false)
	c.Params = gin.Params{{Key: "username", Value: "alice_01"}}

	h.GetByUsername(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
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
	c, w := newTestContext(http.MethodGet, "/users/nobody", nil, uuid.Nil, false)
	c.Params = gin.Params{{Key: "username", Value: "nobody"}}

	h.GetByUsername(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestUpdateProfile_Unauthenticated(t *testing.T) {
	h := &UserHandler{}
	c, w := newTestContext(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{}`)), uuid.Nil, false)

	h.UpdateProfile(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestUpdateProfile_NoFields(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{}
	c, w := newTestContext(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{}`)), me, true)

	h.UpdateProfile(c)

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
	c, w := newTestContext(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"display_name":"Alicia"}`)), me, true)

	h.UpdateProfile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if fake.gotCmd.UserID != me {
		t.Fatalf("UserID = %v, want %v", fake.gotCmd.UserID, me)
	}
	if fake.gotCmd.DisplayName == nil || *fake.gotCmd.DisplayName != "Alicia" {
		t.Fatalf("DisplayName = %v, want Alicia", fake.gotCmd.DisplayName)
	}
}

func TestUpdateProfile_UsernameTakenMapsTo409(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{updateProfile: &fakeUpdateProfile{err: domain.ErrUsernameTaken}}
	c, w := newTestContext(http.MethodPatch, "/users/me", bytes.NewReader([]byte(`{"username":"taken"}`)), me, true)

	h.UpdateProfile(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

// multipartAvatarBody builds a multipart/form-data body with one "avatar"
// file part containing data. The handler under test sniffs the actual
// image content type from data's magic bytes server-side — this helper
// does not need to be told what type data is.
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
	body, ct := multipartAvatarBody(t, pngMagicBytes)
	c, w := newTestContext(http.MethodPost, "/users/me/avatar", body, me, true)
	c.Request.Header.Set("Content-Type", ct)

	h.UploadAvatar(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestUploadAvatar_RejectsUnknownType(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &UserHandler{uploadAvatar: &fakeUploadAvatar{}}
	body, ct := multipartAvatarBody(t, []byte("not an image"))
	c, w := newTestContext(http.MethodPost, "/users/me/avatar", body, me, true)
	c.Request.Header.Set("Content-Type", ct)

	h.UploadAvatar(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

type closingReader struct{ *bytes.Reader }

func (closingReader) Close() error { return nil }

func TestServeAvatar_Found(t *testing.T) {
	h := &UserHandler{serveAvatar: &fakeServeAvatar{data: closingReader{bytes.NewReader([]byte("png-bytes"))}, ct: "image/png"}}
	c, w := newTestContext(http.MethodGet, "/users/alice_01/avatar", nil, uuid.Nil, false)
	c.Params = gin.Params{{Key: "username", Value: "alice_01"}}

	h.ServeAvatar(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestServeAvatar_NotFound(t *testing.T) {
	h := &UserHandler{serveAvatar: &fakeServeAvatar{err: errors.New("not found")}}
	c, w := newTestContext(http.MethodGet, "/users/alice_01/avatar", nil, uuid.Nil, false)
	c.Params = gin.Params{{Key: "username", Value: "alice_01"}}

	h.ServeAvatar(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
