package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainconv "pet-messenger/internal/domain/conversation"
	usecaseconv "pet-messenger/internal/usecase/conversation"
)

// conversationService is satisfied by *usecaseconv.Service; declared here so
// tests can inject a fake without a database.
type conversationService interface {
	OpenDirect(ctx context.Context, me, other uuid.UUID) (*domainconv.Conversation, error)
	CreateGroupChat(ctx context.Context, creator uuid.UUID, title string) (*domainconv.Conversation, error)
	ListMine(ctx context.Context, me uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
	CreateChannel(ctx context.Context, creator uuid.UUID, kind domainconv.Kind, title string, public bool) (*domainconv.Conversation, error)
	JoinChannel(ctx context.Context, actor, channelID uuid.UUID) (*domainconv.Conversation, error)
	LeaveChannel(ctx context.Context, actor, channelID uuid.UUID) error
	ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
}

type ConversationHandler struct {
	svc conversationService
}

func NewConversationHandler(svc *usecaseconv.Service) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

// RegisterRoutes mounts the conversation routes on rg. rg is expected to already
// carry AuthMiddleware, so every handler can rely on ContextUserIDKey.
func (h *ConversationHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/direct", h.OpenDirect)
	rg.POST("/groups", h.CreateGroupChat)
	rg.POST("/channels", h.CreateChannel)
	rg.GET("/channels", h.ListPublicChannels)
	rg.GET("", h.ListMine)
	rg.POST("/:id/join", h.JoinChannel)
	rg.DELETE("/:id/members/me", h.LeaveChannel)
}

// actorID reads the authenticated user id that AuthMiddleware stored on the
// context.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

type openDirectRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type createGroupRequest struct {
	Title string `json:"title" binding:"required"`
}

type createChannelRequest struct {
	Kind   string `json:"kind" binding:"required"`
	Title  string `json:"title" binding:"required"`
	Public bool   `json:"public"`
}

type conversationResponse struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Title         string    `json:"title"`
	AvatarURL     string    `json:"avatar_url"`
	Public        bool      `json:"public"`
	CreatedAt     time.Time `json:"created_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type conversationListResponse struct {
	Conversations []conversationResponse `json:"conversations"`
	NextCursor    string                 `json:"next_cursor,omitempty"`
}

type channelListResponse struct {
	Channels   []conversationResponse `json:"channels"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func toConversationResponse(c *domainconv.Conversation) conversationResponse {
	return conversationResponse{
		ID:            c.ID.String(),
		Kind:          string(c.Kind),
		Title:         c.Title,
		AvatarURL:     c.AvatarURL,
		Public:        c.Public,
		CreatedAt:     c.CreatedAt,
		LastMessageAt: c.LastMessageAt,
	}
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecaseconv.ErrCannotDirectSelf),
		errors.Is(err, domainconv.ErrInvalidDirectMembers),
		errors.Is(err, domainconv.ErrInvalidTitle),
		errors.Is(err, domainconv.ErrInvalidCreator),
		errors.Is(err, domainconv.ErrInvalidKind):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, usecaseconv.ErrRecipientNotFound),
		errors.Is(err, domainconv.ErrNotChannel),
		errors.Is(err, domainconv.ErrNotFound),
		errors.Is(err, domainconv.ErrNotMember):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

func (h *ConversationHandler) OpenDirect(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req openDirectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	other, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is not a valid uuid"})
		return
	}
	conv, err := h.svc.OpenDirect(c.Request.Context(), me, other)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toConversationResponse(conv))
}

func (h *ConversationHandler) CreateGroupChat(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conv, err := h.svc.CreateGroupChat(c.Request.Context(), me, req.Title)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.Header("Location", "/conversations/"+conv.ID.String())
	c.JSON(http.StatusCreated, toConversationResponse(conv))
}

func (h *ConversationHandler) ListMine(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	before, beforeID, err := decodeConversationCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = n
	}

	list, err := h.svc.ListMine(c.Request.Context(), me, before, beforeID, limit)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	resp := conversationListResponse{Conversations: make([]conversationResponse, 0, len(list))}
	for _, cv := range list {
		resp.Conversations = append(resp.Conversations, toConversationResponse(cv))
	}
	// A page that is exactly the effective limit may have more behind it; hand
	// back the keyset of the last row so the client can ask for the next page.
	if eff := usecaseconv.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeConversationCursor(last.LastMessageAt, last.ID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ConversationHandler) CreateChannel(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req createChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conv, err := h.svc.CreateChannel(c.Request.Context(), me, domainconv.Kind(req.Kind), req.Title, req.Public)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.Header("Location", "/conversations/"+conv.ID.String())
	c.JSON(http.StatusCreated, toConversationResponse(conv))
}

func (h *ConversationHandler) ListPublicChannels(c *gin.Context) {
	if _, ok := actorID(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	before, beforeID, err := decodeConversationCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = n
	}

	// Bound a pathological %-heavy ILIKE pattern: truncate rather than 400.
	q := c.Query("q")
	if len(q) > 100 {
		q = q[:100]
	}

	list, err := h.svc.ListPublicChannels(c.Request.Context(), q, before, beforeID, limit)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	resp := channelListResponse{Channels: make([]conversationResponse, 0, len(list))}
	for _, cv := range list {
		resp.Channels = append(resp.Channels, toConversationResponse(cv))
	}
	if eff := usecaseconv.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeConversationCursor(last.LastMessageAt, last.ID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ConversationHandler) JoinChannel(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	conv, err := h.svc.JoinChannel(c.Request.Context(), me, id)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toConversationResponse(conv))
}

func (h *ConversationHandler) LeaveChannel(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	if err := h.svc.LeaveChannel(c.Request.Context(), me, id); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
