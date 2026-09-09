package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pet-messenger/internal/domain/pushtoken"
)

type PushTokenHandler struct {
	repo pushtoken.Repository
}

func NewPushTokenHandler(repo pushtoken.Repository) *PushTokenHandler {
	return &PushTokenHandler{repo: repo}
}

// RegisterRoutes mounts the push-token routes on rg. rg is expected to already
// carry AuthMiddleware, so every handler can rely on ContextUserIDKey.
func (h *PushTokenHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.Register)
	rg.DELETE("", h.Unregister)
}

type registerPushTokenRequest struct {
	Platform string `json:"platform" binding:"required"`
	Token    string `json:"token" binding:"required"`
}

type deletePushTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

type pushTokenResponse struct {
	ID         string    `json:"id"`
	Platform   string    `json:"platform"`
	Token      string    `json:"token"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

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
	tok, err := pushtoken.New(me, pushtoken.Platform(req.Platform), req.Token)
	if err != nil {
		if errors.Is(err, pushtoken.ErrInvalidPlatform) || errors.Is(err, pushtoken.ErrInvalidToken) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	stored, err := h.repo.Upsert(c.Request.Context(), tok)
	if err != nil {
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
	// DeleteByToken treats a missing row as a no-op, so this is idempotent.
	if err := h.repo.DeleteByToken(c.Request.Context(), me, req.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}
