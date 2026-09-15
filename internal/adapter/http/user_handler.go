package http

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"pet-messenger/internal/domain/user"
	"pet-messenger/internal/infrastructure/objectstore"
)

// UserHandler exposes user profile lookup, profile updates, and avatar
// upload/serving.
type UserHandler struct {
	users   user.Repository
	avatars objectstore.AvatarStore
}

func NewUserHandler(users user.Repository, avatars objectstore.AvatarStore) *UserHandler {
	return &UserHandler{users: users, avatars: avatars}
}

func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:username", h.GetByUsername)
	rg.GET("/:username/avatar", h.ServeAvatar)
	rg.PATCH("/me", h.UpdateProfile)
	rg.POST("/me/avatar", h.UploadAvatar)
}

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

type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Username    *string `json:"username"`
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DisplayName == nil && req.Username == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}
	u, err := h.users.GetByID(c.Request.Context(), me)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if req.DisplayName != nil {
		if err := u.SetDisplayName(*req.DisplayName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if req.Username != nil {
		if err := u.Rename(*req.Username); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.users.Update(c.Request.Context(), u); err != nil {
		if errors.Is(err, user.ErrUsernameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toUserProfileResponse(u))
}

const maxAvatarBytes = 2 << 20 // 2MiB

func (h *UserHandler) UploadAvatar(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes)
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar file is required"})
		return
	}
	defer file.Close()

	sniff := make([]byte, 512)
	n, _ := io.ReadFull(file, sniff)
	contentType := http.DetectContentType(sniff[:n])
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar must be png, jpeg, or webp"})
		return
	}
	full := io.MultiReader(bytes.NewReader(sniff[:n]), file)

	u, err := h.users.GetByID(c.Request.Context(), me)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if err := h.avatars.Put(c.Request.Context(), me, contentType, full, header.Size); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	u.AvatarURL = "/users/" + u.Username + "/avatar"
	if err := h.users.Update(c.Request.Context(), u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toUserProfileResponse(u))
}

func (h *UserHandler) ServeAvatar(c *gin.Context) {
	u, err := h.users.GetByUsername(c.Request.Context(), c.Param("username"))
	if err != nil || u.AvatarURL == "" {
		c.Status(http.StatusNotFound)
		return
	}
	data, contentType, err := h.avatars.Get(c.Request.Context(), u.ID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer data.Close()
	c.Header("Cache-Control", "private, max-age=300")
	// The content-type served here is whatever was sniffed and stored at
	// upload time (already restricted to png/jpeg/webp), but nosniff still
	// costs nothing and stops a browser from second-guessing it.
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, -1, contentType, data, nil)
}
