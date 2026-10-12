package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/domain"
)

// fakeOpenDirect is a minimal in.OpenDirectUseCase stand-in.
type fakeOpenDirect struct {
	got in.OpenDirectCommand
	ret *domain.Conversation
	err error
}

func (f *fakeOpenDirect) Handle(_ context.Context, cmd in.OpenDirectCommand) (*domain.Conversation, error) {
	f.got = cmd
	return f.ret, f.err
}

func newTestRouter(t *testing.T, openDirect in.OpenDirectUseCase) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(httpadapter.ContextUserIDKey, uuid.Must(uuid.NewV7()))
		c.Next()
	})
	// NewConversationHandler takes 13 use cases; this test wires only the
	// one under test (openDirect) and passes nil for the rest — safe here
	// because the only route exercised is OpenDirect, which never touches
	// an unused dependency.
	h := NewConversationHandler(openDirect, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	rg := r.Group("/conversations")
	h.RegisterRoutes(rg)
	return r
}

func TestOpenDirect_ReturnsConversationJSON(t *testing.T) {
	other := uuid.Must(uuid.NewV7())
	fake := &fakeOpenDirect{ret: &domain.Conversation{ID: uuid.Must(uuid.NewV7()), Kind: domain.KindDirect}}
	r := newTestRouter(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/conversations/direct", strings.NewReader(`{"user_id":"`+other.String()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if fake.got.Other != other {
		t.Fatalf("Other = %s, want %s", fake.got.Other, other)
	}
}

func TestOpenDirect_WritesErrorMapping(t *testing.T) {
	other := uuid.Must(uuid.NewV7())
	fake := &fakeOpenDirect{err: command.ErrCannotDirectSelf}
	r := newTestRouter(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/conversations/direct", strings.NewReader(`{"user_id":"`+other.String()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}
