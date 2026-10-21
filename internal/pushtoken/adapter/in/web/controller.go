// internal/pushtoken/adapter/in/web/controller.go

// Package web is the pushtoken context's primary ("driving") HTTP adapter:
// it translates gin requests into port/in calls and port/in results into
// the same JSON shapes the pre-retrofit
// internal/adapter/http.PushTokenHandler produced.
package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	"pet-messenger/internal/pushtoken/application/port/in"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported) from internal/adapter/http. It is
// unexported there, matching the precedent every other context's own
// controller already follows.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// PushTokenHandler is the primary HTTP adapter for the pushtoken context.
type PushTokenHandler struct {
	register   in.RegisterPushTokenUseCase
	unregister in.UnregisterPushTokenUseCase
}

// NewPushTokenHandler wires the pushtoken HTTP routes to their use cases.
func NewPushTokenHandler(register in.RegisterPushTokenUseCase, unregister in.UnregisterPushTokenUseCase) *PushTokenHandler {
	return &PushTokenHandler{register: register, unregister: unregister}
}

// RegisterRoutes mounts the push-token routes on rg. rg is expected to
// already carry AuthMiddleware, so every handler can rely on
// ContextUserIDKey.
func (h *PushTokenHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.Register)
	rg.DELETE("", h.Unregister)
}

// registerPushTokenRequest is the request body for Register.
type registerPushTokenRequest struct {
	Platform string `json:"platform" binding:"required"`
	Token    string `json:"token" binding:"required"`
}

// deletePushTokenRequest is the request body for Unregister.
type deletePushTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// pushTokenResponse is the JSON representation of a registered push token.
type pushTokenResponse struct {
	ID         string    `json:"id"`
	Platform   string    `json:"platform"`
	Token      string    `json:"token"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// Register handles POST requests to register (or refresh) the
// authenticated actor's device push endpoint.
func (h *PushTokenHandler) Register(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req registerPushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stored, err := h.register.Handle(c.Request.Context(), in.RegisterPushTokenCommand{
		Actor: me, Platform: domainpush.Platform(req.Platform), Token: req.Token,
	})
	if err != nil {
		if errors.Is(err, domainpush.ErrInvalidPlatform) || errors.Is(err, domainpush.ErrInvalidToken) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, pushTokenResponse{
		ID:         stored.ID.String(),
		Platform:   string(stored.Platform),
		Token:      stored.Token,
		CreatedAt:  stored.CreatedAt,
		LastUsedAt: stored.LastUsedAt,
	})
}

// Unregister handles DELETE requests to remove the authenticated actor's
// device push endpoint.
func (h *PushTokenHandler) Unregister(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req deletePushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.unregister.Handle(c.Request.Context(), in.UnregisterPushTokenCommand{Actor: me, Token: req.Token}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}
