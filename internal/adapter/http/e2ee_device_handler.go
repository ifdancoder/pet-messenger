package http

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/e2ee"
)

type E2EEDeviceHandler struct {
	repo          e2ee.Repository
	conversations conversation.Repository
}

func NewE2EEDeviceHandler(repo e2ee.Repository, conversations conversation.Repository) *E2EEDeviceHandler {
	return &E2EEDeviceHandler{repo: repo, conversations: conversations}
}

func (h *E2EEDeviceHandler) RegisterConversationRoutes(rg *gin.RouterGroup) {
	rg.GET("/:id/e2ee/devices", h.ListConversationDevices)
}

func (h *E2EEDeviceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/me/e2ee/devices", h.Register)
	rg.GET("/me/e2ee/devices", h.List)
	rg.POST("/me/e2ee/devices/:id/approve", h.Approve)
}

type registerE2EEDeviceRequest struct {
	ID               string `json:"id" binding:"required"`
	PublicKey        string `json:"public_key" binding:"required"`
	SigningPublicKey string `json:"signing_public_key" binding:"required"`
	Label            string `json:"label" binding:"max=120"`
}
type e2eeDeviceResponse struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id,omitempty"`
	PublicKey   string    `json:"public_key,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func e2eeDeviceDTO(d *e2ee.Device) e2eeDeviceResponse {
	return e2eeDeviceResponse{
		ID: d.ID.String(), UserID: d.UserID.String(), Fingerprint: d.Fingerprint,
		Label: d.Label, Status: string(d.Status), CreatedAt: d.CreatedAt,
	}
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
	signingKey, err := base64.RawURLEncoding.DecodeString(req.SigningPublicKey)
	if err != nil || len(signingKey) != ed25519.PublicKeySize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "signing public key must be a 32-byte base64url value"})
		return
	}
	existing, err := h.repo.Get(c.Request.Context(), id)
	if err == nil {
		if existing.UserID != actor || !bytes.Equal(existing.PublicKey, key) || !bytes.Equal(existing.SigningPublicKey, signingKey) {
			c.JSON(http.StatusConflict, gin.H{"error": "device id is already registered"})
			return
		}
		c.JSON(http.StatusOK, e2eeDeviceDTO(existing))
		return
	}
	if !errors.Is(err, e2ee.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	fingerprintMaterial := append(append(make([]byte, 0, len(key)+len(signingKey)), key...), signingKey...)
	sum := sha256.Sum256(fingerprintMaterial)
	d := &e2ee.Device{ID: id, UserID: actor, PublicKey: key, SigningPublicKey: signingKey, Fingerprint: hex.EncodeToString(sum[:]), Label: req.Label, CreatedAt: time.Now().UTC()}
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
	devices, err := h.repo.ListForUser(c.Request.Context(), actor)
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

func (h *E2EEDeviceHandler) ListConversationDevices(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	conversationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	if _, err := h.conversations.GetMember(c.Request.Context(), conversationID, actor); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a conversation member"})
		return
	}
	devices, err := h.repo.ListTrustedForConversation(c.Request.Context(), conversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	out := make([]e2eeDeviceResponse, len(devices))
	for i, d := range devices {
		out[i] = e2eeDeviceDTO(d)
		out[i].PublicKey = base64.RawURLEncoding.EncodeToString(d.PublicKey)
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

type approveE2EEDeviceRequest struct {
	ApproverID string `json:"approver_id" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

func approvalStatement(approverID, targetID uuid.UUID, targetFingerprint string) []byte {
	return []byte("pet-messenger:approve-device:v1:" + approverID.String() + ":" + targetID.String() + ":" + targetFingerprint)
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
	var req approveE2EEDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	approver, err := uuid.Parse(req.ApproverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trusted device id is required"})
		return
	}
	signature, err := base64.RawURLEncoding.DecodeString(req.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval signature"})
		return
	}
	d, err := h.repo.Get(c.Request.Context(), approver)
	if err != nil || d.UserID != actor || d.Status != e2ee.StatusTrusted || len(d.SigningPublicKey) != ed25519.PublicKeySize {
		c.JSON(http.StatusForbidden, gin.H{"error": "device is not trusted"})
		return
	}
	targetDevice, err := h.repo.Get(c.Request.Context(), target)
	if err != nil || targetDevice.UserID != actor || targetDevice.Status != e2ee.StatusPending {
		c.JSON(http.StatusNotFound, gin.H{"error": "pending device not found"})
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(d.SigningPublicKey), approvalStatement(approver, target, targetDevice.Fingerprint), signature) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid approval signature"})
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
