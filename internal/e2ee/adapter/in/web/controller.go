// internal/e2ee/adapter/in/web/controller.go

// Package web is the e2ee context's primary ("driving") HTTP adapter: it
// translates gin requests into port/in calls and port/in results into the
// same JSON shapes the pre-retrofit internal/adapter/http.E2EEDeviceHandler
// produced.
package web

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	"pet-messenger/internal/e2ee/application/command"
	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/query"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// E2EEDeviceHandler is the primary adapter for every e2ee route. Each
// field is the one-method use-case interface the matching handler method
// calls.
type E2EEDeviceHandler struct {
	register                in.RegisterDeviceUseCase
	approve                 in.ApproveDeviceUseCase
	putCallKeyEnvelopes     in.PutCallKeyEnvelopesUseCase
	listDevices             in.ListDevicesUseCase
	listConversationDevices in.ListConversationDevicesUseCase
	getCallKeyEnvelope      in.GetCallKeyEnvelopeUseCase
}

// NewE2EEDeviceHandler wires the handler against all six use cases.
func NewE2EEDeviceHandler(register in.RegisterDeviceUseCase, approve in.ApproveDeviceUseCase, putCallKeyEnvelopes in.PutCallKeyEnvelopesUseCase, listDevices in.ListDevicesUseCase, listConversationDevices in.ListConversationDevicesUseCase, getCallKeyEnvelope in.GetCallKeyEnvelopeUseCase) *E2EEDeviceHandler {
	return &E2EEDeviceHandler{register: register, approve: approve, putCallKeyEnvelopes: putCallKeyEnvelopes, listDevices: listDevices, listConversationDevices: listConversationDevices, getCallKeyEnvelope: getCallKeyEnvelope}
}

// RegisterCallRoutes mounts the two call-scoped e2ee routes on rg (the
// /calls group, which already carries AuthMiddleware).
func (h *E2EEDeviceHandler) RegisterCallRoutes(rg *gin.RouterGroup) {
	rg.PUT("/:id/e2ee/key-envelopes", h.PutCallKeyEnvelopes)
	rg.GET("/:id/e2ee/key-envelope", h.GetCallKeyEnvelope)
}

// RegisterConversationRoutes mounts the one conversation-scoped e2ee route
// on rg (the /conversations group).
func (h *E2EEDeviceHandler) RegisterConversationRoutes(rg *gin.RouterGroup) {
	rg.GET("/:id/e2ee/devices", h.ListConversationDevices)
}

// RegisterRoutes mounts the three account-scoped e2ee routes on rg (the
// /users group).
func (h *E2EEDeviceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/me/e2ee/devices", h.Register)
	rg.GET("/me/e2ee/devices", h.List)
	rg.POST("/me/e2ee/devices/:id/approve", h.Approve)
}

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported -- it is unexported in that package)
// from internal/adapter/http, matching the established precedent in every
// other context's own controller.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// registerE2EEDeviceRequest is the Register request body.
type registerE2EEDeviceRequest struct {
	ID               string `json:"id" binding:"required"`
	PublicKey        string `json:"public_key" binding:"required"`
	SigningPublicKey string `json:"signing_public_key" binding:"required"`
	Label            string `json:"label" binding:"max=120"`
}

// e2eeDeviceResponse is the client-visible shape of one device. PublicKey
// is only populated by ListConversationDevices; Register, List and Approve
// all omit it (via omitempty).
type e2eeDeviceResponse struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id,omitempty"`
	PublicKey   string    `json:"public_key,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// toE2EEDeviceResponse builds the common part of a device response.
// Callers that need PublicKey populated (ListConversationDevices) set it
// afterward.
func toE2EEDeviceResponse(d *domaine2ee.Device) e2eeDeviceResponse {
	return e2eeDeviceResponse{
		ID: d.ID.String(), UserID: d.UserID.String(), Fingerprint: d.Fingerprint,
		Label: d.Label, Status: string(d.Status), CreatedAt: d.CreatedAt,
	}
}

// Register registers a new e2ee device for the authenticated user, or
// returns their own already-registered device unchanged if the same
// id/keys were submitted again.
// Register godoc
//
//	@Summary		Register an e2ee device
//	@Description	Registers a new e2ee device for the authenticated user, or returns their own already-registered device unchanged if the same id/keys were submitted again.
//	@Tags			e2ee
//	@Accept			json
//	@Produce		json
//	@Param			request	body		registerE2EEDeviceRequest	true	"Device id and public keys"
//	@Success		200		{object}	e2eeDeviceResponse			"already registered, unchanged"
//	@Success		201		{object}	e2eeDeviceResponse			"newly registered"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		409		{object}	map[string]string	"device id conflict or already registered"
//	@Security		BearerAuth
//	@Router			/users/me/e2ee/devices [post]
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
	d, created, err := h.register.Handle(c.Request.Context(), in.RegisterDeviceCommand{Actor: actor, DeviceID: id, PublicKey: key, SigningPublicKey: signingKey, Label: req.Label})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrDeviceIDConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "device id is already registered"})
		case errors.Is(err, command.ErrDeviceAlreadyRegistered):
			c.JSON(http.StatusConflict, gin.H{"error": "device is already registered"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, toE2EEDeviceResponse(d))
}

// List returns every non-revoked device belonging to the authenticated
// user's own account.
// List godoc
//
//	@Summary		List my e2ee devices
//	@Description	Returns every non-revoked device belonging to the authenticated user's own account.
//	@Tags			e2ee
//	@Produce		json
//	@Success		200	{object}	map[string][]e2eeDeviceResponse
//	@Failure		401	{object}	map[string]string	"unauthenticated"
//	@Security		BearerAuth
//	@Router			/users/me/e2ee/devices [get]
func (h *E2EEDeviceHandler) List(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	devices, err := h.listDevices.Handle(c.Request.Context(), in.ListDevicesQuery{Actor: actor})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	out := make([]e2eeDeviceResponse, len(devices))
	for i, d := range devices {
		out[i] = toE2EEDeviceResponse(d)
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

// ListConversationDevices returns every trusted device belonging to any
// member of the route's conversation, for a caller who is themselves a
// member. Unlike List, the public key is included here -- the client
// needs it to encrypt a new message's key to every trusted recipient
// device.
// ListConversationDevices godoc
//
//	@Summary		List trusted devices in a conversation
//	@Description	Returns every trusted device belonging to any member of the route's conversation, for a caller who is themselves a member. Unlike List, the public key is included -- the client needs it to encrypt a new message's key to every trusted recipient device.
//	@Tags			e2ee
//	@Produce		json
//	@Param			id	path		string	true	"Conversation ID"
//	@Success		200	{object}	map[string][]e2eeDeviceResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string	"unauthenticated"
//	@Failure		403	{object}	map[string]string	"not a conversation member"
//	@Security		BearerAuth
//	@Router			/conversations/{id}/e2ee/devices [get]
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
	devices, err := h.listConversationDevices.Handle(c.Request.Context(), in.ListConversationDevicesQuery{Actor: actor, ConversationID: conversationID})
	if err != nil {
		switch {
		case errors.Is(err, query.ErrNotConversationMember):
			c.JSON(http.StatusForbidden, gin.H{"error": "not a conversation member"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	out := make([]e2eeDeviceResponse, len(devices))
	for i, d := range devices {
		out[i] = toE2EEDeviceResponse(d)
		out[i].PublicKey = base64.RawURLEncoding.EncodeToString(d.PublicKey)
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

// approveE2EEDeviceRequest is the Approve request body.
type approveE2EEDeviceRequest struct {
	ApproverID string `json:"approver_id" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

// Approve trusts the authenticated user's own pending device on the
// vouching of one of their already-trusted devices.
// Approve godoc
//
//	@Summary		Approve a pending e2ee device
//	@Description	Trusts the authenticated user's own pending device on the vouching of one of their already-trusted devices.
//	@Tags			e2ee
//	@Accept			json
//	@Param			id		path	string						true	"Pending device ID"
//	@Param			request	body	approveE2EEDeviceRequest	true	"Approving device id and signature"
//	@Success		204		"no body"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"approver not trusted, invalid signature, or cannot approve"
//	@Failure		404		{object}	map[string]string	"pending device not found"
//	@Security		BearerAuth
//	@Router			/users/me/e2ee/devices/{id}/approve [post]
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
	err = h.approve.Handle(c.Request.Context(), in.ApproveDeviceCommand{Actor: actor, ApproverID: approver, TargetID: target, Signature: signature})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrApproverNotTrusted):
			c.JSON(http.StatusForbidden, gin.H{"error": "device is not trusted"})
		case errors.Is(err, command.ErrPendingDeviceNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "pending device not found"})
		case errors.Is(err, command.ErrInvalidApprovalSignature):
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid approval signature"})
		case errors.Is(err, command.ErrCannotApprove):
			c.JSON(http.StatusForbidden, gin.H{"error": "device cannot approve this request"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

// putCallKeyEnvelope is one envelope entry within a
// putCallKeyEnvelopesRequest.
type putCallKeyEnvelope struct {
	RecipientDeviceID string `json:"recipient_device_id" binding:"required"`
	Ciphertext        string `json:"ciphertext" binding:"required"`
}

// putCallKeyEnvelopesRequest is the PutCallKeyEnvelopes request body.
type putCallKeyEnvelopesRequest struct {
	OwnerDeviceID string               `json:"owner_device_id" binding:"required"`
	Envelopes     []putCallKeyEnvelope `json:"envelopes" binding:"required,max=500,dive"`
}

// PutCallKeyEnvelopes publishes the route's call's encrypted session key,
// wrapped once for every trusted device in the call's conversation. The
// caller must be the call's initiator.
// PutCallKeyEnvelopes godoc
//
//	@Summary		Publish a call's encrypted session key
//	@Description	Publishes the route's call's encrypted session key, wrapped once for every trusted device in the call's conversation. The caller must be the call's initiator.
//	@Tags			e2ee
//	@Accept			json
//	@Param			id		path	string						true	"Call ID"
//	@Param			request	body	putCallKeyEnvelopesRequest	true	"Owner device id and per-recipient envelopes"
//	@Success		204		"no body"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"not a conversation member, not the initiator, or owner device not trusted"
//	@Failure		404		{object}	map[string]string	"active call not found"
//	@Security		BearerAuth
//	@Router			/calls/{id}/e2ee/key-envelopes [put]
func (h *E2EEDeviceHandler) PutCallKeyEnvelopes(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call id"})
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
	envelopes := make([]in.KeyEnvelopeInput, 0, len(req.Envelopes))
	for _, item := range req.Envelopes {
		recipientID, err := uuid.Parse(item.RecipientDeviceID)
		ciphertext, decodeErr := base64.RawURLEncoding.DecodeString(item.Ciphertext)
		if err != nil || decodeErr != nil || len(ciphertext) == 0 || len(ciphertext) > 4096 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call key envelope"})
			return
		}
		envelopes = append(envelopes, in.KeyEnvelopeInput{RecipientDeviceID: recipientID, Ciphertext: ciphertext})
	}
	err = h.putCallKeyEnvelopes.Handle(c.Request.Context(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: callID, OwnerDeviceID: ownerID, Envelopes: envelopes})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrCallNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "active call not found"})
		case errors.Is(err, command.ErrNotConversationMember):
			c.JSON(http.StatusForbidden, gin.H{"error": "not a conversation member"})
		case errors.Is(err, command.ErrNotCallInitiator):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the call initiator can publish its key"})
		case errors.Is(err, command.ErrOwnerNotTrusted):
			c.JSON(http.StatusForbidden, gin.H{"error": "owner device is not trusted"})
		case errors.Is(err, command.ErrInvalidKeyEnvelope):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call key envelope"})
		case errors.Is(err, command.ErrMissingKeyEnvelope):
			c.JSON(http.StatusBadRequest, gin.H{"error": "a call key envelope is required for every trusted device"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

// GetCallKeyEnvelope returns the route's call's encrypted session key,
// wrapped for the caller's own recipient device, once the owning device
// has published it.
// GetCallKeyEnvelope godoc
//
//	@Summary		Get my wrapped copy of a call's session key
//	@Description	Returns the route's call's encrypted session key, wrapped for the caller's own recipient device, once the owning device has published it.
//	@Tags			e2ee
//	@Produce		json
//	@Param			id			path		string	true	"Call ID"
//	@Param			device_id	query		string	true	"Caller's recipient device ID"
//	@Success		200			{object}	map[string]string
//	@Failure		400			{object}	map[string]string
//	@Failure		401			{object}	map[string]string	"unauthenticated"
//	@Failure		403			{object}	map[string]string	"not a conversation member, or device not trusted"
//	@Failure		404			{object}	map[string]string	"active call not found, or key not ready"
//	@Failure		409			{object}	map[string]string	"call key owner is no longer trusted"
//	@Security		BearerAuth
//	@Router			/calls/{id}/e2ee/key-envelope [get]
func (h *E2EEDeviceHandler) GetCallKeyEnvelope(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid call id"})
		return
	}
	recipientID, err := uuid.Parse(c.Query("device_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device id"})
		return
	}
	envelope, owner, err := h.getCallKeyEnvelope.Handle(c.Request.Context(), in.GetCallKeyEnvelopeQuery{Actor: actor, CallID: callID, RecipientDeviceID: recipientID})
	if err != nil {
		switch {
		case errors.Is(err, query.ErrCallNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "active call not found"})
		case errors.Is(err, query.ErrNotConversationMember):
			c.JSON(http.StatusForbidden, gin.H{"error": "not a conversation member"})
		case errors.Is(err, query.ErrRecipientNotTrusted):
			c.JSON(http.StatusForbidden, gin.H{"error": "device is not trusted"})
		case errors.Is(err, query.ErrKeyNotReady):
			c.JSON(http.StatusNotFound, gin.H{"error": "call key is not ready"})
		case errors.Is(err, query.ErrOwnerNoLongerTrusted):
			c.JSON(http.StatusConflict, gin.H{"error": "call key owner is no longer trusted"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"owner_device_id":  envelope.OwnerDeviceID.String(),
		"owner_public_key": base64.RawURLEncoding.EncodeToString(owner.PublicKey),
		"ciphertext":       base64.RawURLEncoding.EncodeToString(envelope.Ciphertext),
	})
}
