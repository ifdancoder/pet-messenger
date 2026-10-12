// internal/adapter/http/http_test_helpers_test.go
//
// doConvJSON is a generic "fire one JSON request at a gin.Engine" helper
// shared by several handlers' tests in this package (call, notification,
// push-token). It used to live in conversation_handler_test.go, deleted
// along with the pre-retrofit ConversationHandler.
package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func doConvJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
