package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/e2ee"
)

type E2EEDeviceHandler struct{ repo e2ee.Repository }

func NewE2EEDeviceHandler(repo e2ee.Repository) *E2EEDeviceHandler {
	return &E2EEDeviceHandler{repo: repo}
}

func (h *E2EEDeviceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/me/e2ee/devices", h.Register)
	rg.GET("/me/e2ee/devices", h.List)
	rg.POST("/me/e2ee/devices/:id/approve", h.Approve)
}

type registerE2EEDeviceRequest struct {
	ID        string `json:"id" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
	Label     string `json:"label" binding:"max=120"`
}
type e2eeDeviceResponse struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func e2eeDeviceDTO(d *e2ee.Device) e2eeDeviceResponse {
	return e2eeDeviceResponse{ID: d.ID.String(), Fingerprint: d.Fingerprint, Label: d.Label, Status: string(d.Status), CreatedAt: d.CreatedAt}
}

func (h *E2EEDeviceHandler) Register(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req registerE2EEDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	key, err := base64.RawURLEncoding.DecodeString(req.PublicKey)
	if err != nil || len(key) != 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "public key must be a 32-byte base64url value"})
		return
	}
	trusted, err := h.repo.ListTrusted(c.Request.Context(), actor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	sum := sha256.Sum256(key)
	status := e2ee.StatusPending
	if len(trusted) == 0 {
		status = e2ee.StatusTrusted
	}
	d := &e2ee.Device{ID: id, UserID: actor, PublicKey: key, Fingerprint: hex.EncodeToString(sum[:]), Label: req.Label, Status: status, CreatedAt: time.Now().UTC()}
	if err := h.repo.Register(c.Request.Context(), d); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "device is already registered"})
		return
	}
	c.JSON(http.StatusCreated, e2eeDeviceDTO(d))
}
func (h *E2EEDeviceHandler) List(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	devices, err := h.repo.ListTrusted(c.Request.Context(), actor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	out := make([]e2eeDeviceResponse, len(devices))
	for i, d := range devices {
		out[i] = e2eeDeviceDTO(d)
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}
func (h *E2EEDeviceHandler) Approve(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	target, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	approver, err := uuid.Parse(c.GetHeader("X-E2EE-Device-ID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trusted device id is required"})
		return
	}
	d, err := h.repo.Get(c.Request.Context(), approver)
	if err != nil || d.UserID != actor {
		c.JSON(http.StatusForbidden, gin.H{"error": "device is not trusted"})
		return
	}
	if err = h.repo.Approve(c.Request.Context(), approver, target); err != nil {
		if errors.Is(err, e2ee.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pending device not found"})
		} else {
			c.JSON(http.StatusForbidden, gin.H{"error": "device cannot approve this request"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
