package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"pet-messenger/internal/domain/user"
)

type userLookupRepository interface {
	GetByUsername(ctx context.Context, username string) (*user.User, error)
}

// UserHandler exposes the minimum user profile lookup needed to open a direct
// conversation. It deliberately accepts an exact username rather than a broad
// directory search.
type UserHandler struct{ users userLookupRepository }

func NewUserHandler(users user.Repository) *UserHandler { return &UserHandler{users: users} }

func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) { rg.GET("/:username", h.GetByUsername) }

type userProfileResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

func toUserProfileResponse(u *user.User) userProfileResponse {
	return userProfileResponse{ID: u.ID.String(), Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}

func (h *UserHandler) GetByUsername(c *gin.Context) {
	u, err := h.users.GetByUsername(c.Request.Context(), c.Param("username"))
	if errors.Is(err, user.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toUserProfileResponse(u))
}
