// internal/adapter/http/middleware.go
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	infraauth "pet-messenger/internal/infrastructure/auth"
)

const ContextUserIDKey = "user_id"

func AuthMiddleware(verifier *infraauth.JWTVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, hasBearer := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !hasBearer {
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		userID, err := verifier.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set(ContextUserIDKey, userID)
		c.Next()
	}
}
