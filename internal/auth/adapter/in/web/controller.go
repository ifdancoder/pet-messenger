// Package web is the auth context's primary ("driving") HTTP adapter: it
// translates gin requests into port/in calls and port/in results into the
// same JSON shapes the pre-retrofit internal/adapter/http.AuthHandler
// produced.
package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"pet-messenger/internal/auth/application/command"
	"pet-messenger/internal/auth/application/port/in"
	domainuser "pet-messenger/internal/user/domain"
)

// AuthHandler is the primary adapter for every /auth route. Each field is
// the one-method use-case interface the matching handler method calls.
type AuthHandler struct {
	register in.RegisterUseCase
	login    in.LoginUseCase
	refresh  in.RefreshUseCase
	logout   in.LogoutUseCase
}

// NewAuthHandler wires the handler against all four use cases.
func NewAuthHandler(register in.RegisterUseCase, login in.LoginUseCase, refresh in.RefreshUseCase, logout in.LogoutUseCase) *AuthHandler {
	return &AuthHandler{register: register, login: login, refresh: refresh, logout: logout}
}

// RegisterRoutes mounts the auth routes on rg. These routes are
// deliberately NOT behind AuthMiddleware: registering, logging in, and
// refreshing all happen before a client has an access token.
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

func toAuthResponse(r *in.Result) authResponse {
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
	case errors.Is(err, domainuser.ErrInvalidUsername), errors.Is(err, domainuser.ErrInvalidDisplayName), errors.Is(err, command.ErrWeakPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domainuser.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, command.ErrInvalidCredentials), errors.Is(err, command.ErrInvalidRefreshToken):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// Register handles POST /auth/register: it creates a new account and
// responds 201 with the same session payload Login would produce.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.register.Handle(c.Request.Context(), in.RegisterCommand{Username: req.Username, Password: req.Password, DisplayName: req.DisplayName})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAuthResponse(result))
}

// Login handles POST /auth/login: it verifies credentials and responds
// 200 with a new session payload.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.login.Handle(c.Request.Context(), in.LoginCommand{Username: req.Username, Password: req.Password, DeviceInfo: req.DeviceInfo})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Refresh handles POST /auth/refresh: it rotates a refresh token and
// responds 200 with a new session payload.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.refresh.Handle(c.Request.Context(), in.RefreshCommand{RefreshToken: req.RefreshToken})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Logout handles POST /auth/logout: it revokes the given refresh token's
// session and responds 204 with no body.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.logout.Handle(c.Request.Context(), in.LogoutCommand{RefreshToken: req.RefreshToken}); err != nil {
		writeAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
