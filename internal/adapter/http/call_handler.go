package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/domain/conversation"
	usecasecall "pet-messenger/internal/usecase/call"
)

type callService interface {
	StartCall(ctx context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, usecasecall.JoinInfo, error)
	JoinCall(ctx context.Context, actorID, callID uuid.UUID, role domaincall.Role) (usecasecall.JoinInfo, error)
	EndCall(ctx context.Context, actorID, callID uuid.UUID) error
	MuteParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error
	UnmuteParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error
	RemoveParticipant(ctx context.Context, actorID, callID, targetID uuid.UUID) error
	GetLiveForConversation(ctx context.Context, actorID, conversationID uuid.UUID) (*domaincall.Call, error)
}

type CallHandler struct{ svc callService }

func NewCallHandler(svc *usecasecall.Service) *CallHandler { return &CallHandler{svc: svc} }

func (h *CallHandler) RegisterConversationRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/calls", h.Start)
	rg.GET("/:id/calls/live", h.Live)
}

func (h *CallHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/join", h.Join)
	rg.POST("/:id/end", h.End)
	rg.POST("/:id/participants/:userId/mute", h.Mute)
	rg.POST("/:id/participants/:userId/unmute", h.Unmute)
	rg.DELETE("/:id/participants/:userId", h.Remove)
}

type callDTO struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	InitiatorID    string     `json:"initiator_id"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
}

func toCallDTO(c *domaincall.Call) callDTO {
	return callDTO{
		ID: c.ID.String(), ConversationID: c.ConversationID.String(),
		InitiatorID: c.InitiatorID.String(), Status: string(c.Status),
		StartedAt: c.StartedAt, EndedAt: c.EndedAt,
	}
}

func (h *CallHandler) Start(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad conversation id"})
		return
	}
	call, join, err := h.svc.StartCall(c.Request.Context(), actor, convID)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"call": toCallDTO(call), "join": join})
}

func (h *CallHandler) Live(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad conversation id"})
		return
	}
	call, err := h.svc.GetLiveForConversation(c.Request.Context(), actor, convID)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"call": toCallDTO(call)})
}

type joinRequest struct {
	Role string `json:"role"`
}

func (h *CallHandler) Join(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad call id"})
		return
	}
	var body joinRequest
	_ = c.ShouldBindJSON(&body) // body optional
	role := domaincall.Role(body.Role)
	if !role.Valid() {
		role = domaincall.RolePublisher
	}
	join, err := h.svc.JoinCall(c.Request.Context(), actor, callID, role)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"join": join})
}

func (h *CallHandler) End(c *gin.Context) {
	h.mutate(c, func(ctx context.Context, actor, callID uuid.UUID) error {
		return h.svc.EndCall(ctx, actor, callID)
	})
}

func (h *CallHandler) Mute(c *gin.Context) {
	h.targeted(c, h.svc.MuteParticipant)
}

func (h *CallHandler) Unmute(c *gin.Context) {
	h.targeted(c, h.svc.UnmuteParticipant)
}

func (h *CallHandler) Remove(c *gin.Context) {
	h.targeted(c, h.svc.RemoveParticipant)
}

func (h *CallHandler) mutate(c *gin.Context, fn func(ctx context.Context, actor, callID uuid.UUID) error) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad call id"})
		return
	}
	if err := fn(c.Request.Context(), actor, callID); err != nil {
		if errors.Is(err, domaincall.ErrAlreadyEnded) {
			c.Status(http.StatusNoContent)
			return
		}
		writeCallError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CallHandler) targeted(c *gin.Context, fn func(ctx context.Context, actor, callID, targetID uuid.UUID) error) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	callID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad call id"})
		return
	}
	target, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad user id"})
		return
	}
	if err := fn(c.Request.Context(), actor, callID, target); err != nil {
		writeCallError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeCallError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecasecall.ErrCallNotSupported), errors.Is(err, usecasecall.ErrModerationNotSupported), errors.Is(err, conversation.ErrConversationArchived), errors.Is(err, usecasecall.ErrCannotTargetSelf):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, usecasecall.ErrForbidden), errors.Is(err, conversation.ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, domaincall.ErrNotFound), errors.Is(err, domaincall.ErrCallNotLive), errors.Is(err, conversation.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, usecasecall.ErrNoMediaNodes), errors.Is(err, usecasecall.ErrMediaUnavailable), errors.Is(err, usecasecall.ErrMediaNodeGone):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domaincall.ErrAlreadyEnded):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
