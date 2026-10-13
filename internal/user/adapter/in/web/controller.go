// internal/user/adapter/in/web/controller.go

// Package web is the user context's primary ("driving") HTTP adapter: it
// translates gin requests into port/in calls and port/in results into the
// same JSON shapes the pre-retrofit internal/adapter/http.UserHandler
// produced.
package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/user/application/port/in"
	"pet-messenger/internal/user/domain"
)

// UserHandler is the primary adapter for every /users route handled by
// this context. Each field is the one-method use-case interface the
// matching handler method calls.
type UserHandler struct {
	getByUsername in.GetByUsernameUseCase
	serveAvatar   in.ServeAvatarUseCase
	updateProfile in.UpdateProfileUseCase
	uploadAvatar  in.UploadAvatarUseCase
}

// NewUserHandler wires the handler against all four use cases.
func NewUserHandler(
	getByUsername in.GetByUsernameUseCase,
	serveAvatar in.ServeAvatarUseCase,
	updateProfile in.UpdateProfileUseCase,
	uploadAvatar in.UploadAvatarUseCase,
) *UserHandler {
	return &UserHandler{getByUsername: getByUsername, serveAvatar: serveAvatar, updateProfile: updateProfile, uploadAvatar: uploadAvatar}
}

// RegisterRoutes mounts the user routes on rg. rg is expected to already
// carry httpadapter.AuthMiddleware, so PATCH/POST handlers can rely on
// httpadapter.ContextUserIDKey.
func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:username", h.GetByUsername)
	rg.GET("/:username/avatar", h.ServeAvatar)
	rg.PATCH("/me", h.UpdateProfile)
	rg.POST("/me/avatar", h.UploadAvatar)
}

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported — it is unexported in that package)
// from internal/adapter/http, matching the existing
// internal/conversation/adapter/in/web precedent.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

type userProfileResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

func toUserProfileResponse(u *domain.User) userProfileResponse {
	return userProfileResponse{ID: u.ID.String(), Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}

func (h *UserHandler) GetByUsername(c *gin.Context) {
	u, err := h.getByUsername.Handle(c.Request.Context(), in.GetByUsernameQuery{Username: c.Param("username")})
	if errors.Is(err, domain.ErrNotFound) {
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

func writeProfileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidDisplayName), errors.Is(err, domain.ErrInvalidUsername):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
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
	u, err := h.updateProfile.Handle(c.Request.Context(), in.UpdateProfileCommand{UserID: me, DisplayName: req.DisplayName, Username: req.Username})
	if err != nil {
		writeProfileError(c, err)
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

	u, err := h.uploadAvatar.Handle(c.Request.Context(), in.UploadAvatarCommand{UserID: me, ContentType: contentType, Data: full, Size: header.Size})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, toUserProfileResponse(u))
}

func (h *UserHandler) ServeAvatar(c *gin.Context) {
	data, contentType, err := h.serveAvatar.Handle(c.Request.Context(), in.ServeAvatarQuery{Username: c.Param("username")})
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
