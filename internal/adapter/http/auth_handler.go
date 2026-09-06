package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pet-messenger/internal/domain/user"
	usecaseauth "pet-messenger/internal/usecase/auth"
)

// authService is satisfied by *usecaseauth.Service; declared here so tests
// can inject a fake without a real database.
type authService interface {
	Register(ctx context.Context, username, password, displayName string) (*usecaseauth.Result, error)
	Login(ctx context.Context, username, password, deviceInfo string) (*usecaseauth.Result, error)
	Refresh(ctx context.Context, refreshToken string) (*usecaseauth.Result, error)
	Logout(ctx context.Context, refreshToken string) error
}

type AuthHandler struct {
	svc authService
}

func NewAuthHandler(svc *usecaseauth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.Register)
	rg.POST("/login", h.Login)
	rg.POST("/refresh", h.Refresh)
	rg.POST("/logout", h.Logout)
}

type registerRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	DeviceInfo string `json:"device_info"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type userResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type authResponse struct {
	User            userResponse `json:"user"`
	AccessToken     string       `json:"access_token"`
	AccessExpiresAt time.Time    `json:"access_token_expires_at"`
	RefreshToken    string       `json:"refresh_token"`
}

func toAuthResponse(r *usecaseauth.Result) authResponse {
	return authResponse{
		User: userResponse{
			ID:          r.User.ID.String(),
			Username:    r.User.Username,
			DisplayName: r.User.DisplayName,
			AvatarURL:   r.User.AvatarURL,
		},
		AccessToken:     r.AccessToken,
		AccessExpiresAt: r.AccessExpiresAt,
		RefreshToken:    r.RefreshToken,
	}
}

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, user.ErrInvalidUsername), errors.Is(err, user.ErrInvalidDisplayName), errors.Is(err, usecaseauth.ErrWeakPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, user.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, usecaseauth.ErrInvalidCredentials), errors.Is(err, usecaseauth.ErrInvalidRefreshToken):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.Register(c.Request.Context(), req.Username, req.Password, req.DisplayName)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAuthResponse(result))
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.Login(c.Request.Context(), req.Username, req.Password, req.DeviceInfo)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		writeAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
