// internal/adapter/http/middleware_test.go
package http

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	infraauth "pet-messenger/internal/infrastructure/auth"
)

func TestAuthMiddleware(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	issuer := infraauth.NewJWTIssuerWithKey(priv, time.Minute)
	verifier := infraauth.NewJWTVerifierWithKey(pub)

	token, _, err := issuer.IssueAccessToken(newTestResult().User.ID)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	r := gin.New()
	r.GET("/protected", AuthMiddleware(verifier), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid token", "Bearer " + token, http.StatusOK},
		{"missing token", "", http.StatusUnauthorized},
		{"invalid token", "Bearer not-a-real-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
