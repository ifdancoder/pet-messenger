// internal/adapter/http/middleware.go
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	infraauth "pet-messenger/internal/infrastructure/auth"
)

const ContextUserIDKey = "user_id"

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Shared by every handler in this package (and, since it is
// unexported, duplicated — not imported — by
// internal/conversation/adapter/in/web, which is a different package).
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// AuthMiddleware authenticates a request from the `Authorization: Bearer <jwt>`
// header only. It is header-only by design: a query-string token
// (`?token=<jwt>`) would be written verbatim into gin's access log and could
// leak into Referer headers and proxy logs, so that fallback is deliberately
// not offered here.
//
// The future WebSocket-upgrade route will need its own query-token-capable
// middleware, because browsers cannot set headers on `new WebSocket(...)`. That
// variant is intentionally NOT added now (YAGNI); it belongs with the route
// that needs it.
func AuthMiddleware(verifier *infraauth.JWTVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
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
