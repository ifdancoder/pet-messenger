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

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/e2ee"
)

type E2EEDeviceHandler struct {
	repo          e2ee.Repository
	conversations conversation.Repository
	calls         domaincall.Repository
}

func NewE2EEDeviceHandler(repo e2ee.Repository, conversations conversation.Repository, calls domaincall.Repository) *E2EEDeviceHandler {
	return &E2EEDeviceHandler{repo: repo, conversations: conversations, calls: calls}
}

func (h *E2EEDeviceHandler) RegisterCallRoutes(rg *gin.RouterGroup) {
	rg.PUT("/:id/e2ee/key-envelopes", h.PutCallKeyEnvelopes)
	rg.GET("/:id/e2ee/key-envelope", h.GetCallKeyEnvelope)
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

type putCallKeyEnvelope struct {
	RecipientDeviceID string `json:"recipient_device_id" binding:"required"`
	Ciphertext        string `json:"ciphertext" binding:"required"`
}

type putCallKeyEnvelopesRequest struct {
	OwnerDeviceID string               `json:"owner_device_id" binding:"required"`
	Envelopes     []putCallKeyEnvelope `json:"envelopes" binding:"required,max=500,dive"`
}

func callKeyScope(callID uuid.UUID) string { return "call:" + callID.String() }

func (h *E2EEDeviceHandler) callForMember(c *gin.Context, actor uuid.UUID) (*domaincall.Call, bool) {
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call id"})
		return nil, false
	}
	call, err := h.calls.GetByID(c.Request.Context(), callID)
	if err != nil || call.Status != domaincall.StatusActive {
		c.JSON(http.StatusNotFound, gin.H{"error": "active call not found"})
		return nil, false
	}
	if _, err := h.conversations.GetMember(c.Request.Context(), call.ConversationID, actor); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a conversation member"})
		return nil, false
	}
	return call, true
}

func (h *E2EEDeviceHandler) PutCallKeyEnvelopes(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	call, ok := h.callForMember(c, actor)
	if !ok {
		return
	}
	if call.InitiatorID != actor {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the call initiator can publish its key"})
		return
	}
	var req putCallKeyEnvelopesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ownerID, err := uuid.Parse(req.OwnerDeviceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid owner device id"})
		return
	}
	owner, err := h.repo.Get(c.Request.Context(), ownerID)
	if err != nil || owner.UserID != actor || owner.Status != e2ee.StatusTrusted {
		c.JSON(http.StatusForbidden, gin.H{"error": "owner device is not trusted"})
		return
	}
	trusted, err := h.repo.ListTrustedForConversation(c.Request.Context(), call.ConversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	allowed := make(map[uuid.UUID]bool, len(trusted))
	for _, device := range trusted {
		allowed[device.ID] = true
	}
	now := time.Now().UTC()
	envelopes := make([]*e2ee.KeyEnvelope, 0, len(req.Envelopes))
	seen := make(map[uuid.UUID]bool, len(req.Envelopes))
	for _, item := range req.Envelopes {
		recipientID, err := uuid.Parse(item.RecipientDeviceID)
		ciphertext, decodeErr := base64.RawURLEncoding.DecodeString(item.Ciphertext)
		if err != nil || decodeErr != nil || len(ciphertext) == 0 || len(ciphertext) > 4096 || !allowed[recipientID] || seen[recipientID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call key envelope"})
			return
		}
		seen[recipientID] = true
		envelopes = append(envelopes, &e2ee.KeyEnvelope{OwnerDeviceID: ownerID, RecipientDeviceID: recipientID, Scope: callKeyScope(call.ID), Ciphertext: ciphertext, CreatedAt: now})
	}
	if len(envelopes) != len(allowed) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a call key envelope is required for every trusted device"})
		return
	}
	if err := h.repo.PutKeyEnvelopes(c.Request.Context(), envelopes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *E2EEDeviceHandler) GetCallKeyEnvelope(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	call, ok := h.callForMember(c, actor)
	if !ok {
		return
	}
	recipientID, err := uuid.Parse(c.Query("device_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	recipient, err := h.repo.Get(c.Request.Context(), recipientID)
	if err != nil || recipient.UserID != actor || recipient.Status != e2ee.StatusTrusted {
		c.JSON(http.StatusForbidden, gin.H{"error": "device is not trusted"})
		return
	}
	envelope, err := h.repo.GetKeyEnvelope(c.Request.Context(), recipientID, callKeyScope(call.ID))
	if errors.Is(err, e2ee.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "call key is not ready"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	owner, err := h.repo.Get(c.Request.Context(), envelope.OwnerDeviceID)
	if err != nil || owner.Status != e2ee.StatusTrusted || owner.UserID != call.InitiatorID {
		c.JSON(http.StatusConflict, gin.H{"error": "call key owner is no longer trusted"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"owner_device_id":  envelope.OwnerDeviceID.String(),
		"owner_public_key": base64.RawURLEncoding.EncodeToString(owner.PublicKey),
		"ciphertext":       base64.RawURLEncoding.EncodeToString(envelope.Ciphertext),
	})
}
