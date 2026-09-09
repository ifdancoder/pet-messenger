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
	LeaveConversation(ctx context.Context, actor, channelID uuid.UUID) error
	ListPublicChannels(ctx context.Context, query string, before time.Time, beforeID uuid.UUID, limit int) ([]*domainconv.Conversation, error)
	AddMember(ctx context.Context, actor, conversationID, invitee uuid.UUID) (*domainconv.Member, error)
	RemoveMember(ctx context.Context, actor, conversationID, target uuid.UUID) error
	SetMemberTier(ctx context.Context, actor, conversationID, target uuid.UUID, tier domainconv.AdminTier) (*domainconv.Member, error)
	ListMembers(ctx context.Context, actor, conversationID uuid.UUID, adminsOnly bool, before time.Time, beforeUserID uuid.UUID, limit int) ([]*domainconv.MemberProfile, error)
	SetArchived(ctx context.Context, actor, conversationID uuid.UUID, archived bool) error
	ClaimCreator(ctx context.Context, actor, conversationID uuid.UUID) (*domainconv.Member, error)
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
	rg.POST("/:id/archive", h.Archive)
	rg.POST("/:id/unarchive", h.Unarchive)
	rg.POST("/:id/claim-creator", h.ClaimCreator)
	rg.POST("/:id/members", h.AddMember)
	rg.GET("/:id/members", h.ListMembers)
	rg.PATCH("/:id/members/me", h.SetOwnTier)
	rg.DELETE("/:id/members/me", h.LeaveConversation)
	rg.PATCH("/:id/members/:user_id", h.SetMemberTier)
	rg.DELETE("/:id/members/:user_id", h.RemoveMember)
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

type addMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// setTierRequest.AdminTier is intentionally not `binding:"required"` — the empty
// string is a valid value meaning "no tier".
type setTierRequest struct {
	AdminTier string `json:"admin_tier"`
}

type memberResponse struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	AdminTier string    `json:"admin_tier"`
	IsCreator bool      `json:"is_creator"`
	JoinedAt  time.Time `json:"joined_at"`
}

func toMemberResponse(m *domainconv.Member) memberResponse {
	return memberResponse{
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		AdminTier: string(m.AdminTier),
		IsCreator: m.IsCreator,
		JoinedAt:  m.JoinedAt,
	}
}

type memberProfileResponse struct {
	memberResponse
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type memberListResponse struct {
	Members    []memberProfileResponse `json:"members"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func toMemberProfileResponse(mp *domainconv.MemberProfile) memberProfileResponse {
	return memberProfileResponse{
		memberResponse: toMemberResponse(&mp.Member),
		Username:       mp.Username,
		DisplayName:    mp.DisplayName,
		AvatarURL:      mp.AvatarURL,
	}
}

type conversationResponse struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	AvatarURL     string     `json:"avatar_url"`
	Public        bool       `json:"public"`
	CreatedAt     time.Time  `json:"created_at"`
	LastMessageAt time.Time  `json:"last_message_at"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
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
		ArchivedAt:    c.ArchivedAt,
	}
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecaseconv.ErrCannotDirectSelf),
		errors.Is(err, domainconv.ErrInvalidDirectMembers),
		errors.Is(err, domainconv.ErrInvalidTitle),
		errors.Is(err, domainconv.ErrInvalidCreator),
		errors.Is(err, domainconv.ErrInvalidKind),
		errors.Is(err, usecaseconv.ErrCannotKickSelf),
		errors.Is(err, usecaseconv.ErrInvalidTier):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domainconv.ErrConversationArchived),
		errors.Is(err, domainconv.ErrCreatorSeatTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, usecaseconv.ErrForbidden),
		errors.Is(err, domainconv.ErrNotCreatorHeir):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, usecaseconv.ErrRecipientNotFound),
		errors.Is(err, usecaseconv.ErrInviteeNotFound),
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

func (h *ConversationHandler) AddMember(c *gin.Context) {
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
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	invitee, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is not a valid uuid"})
		return
	}
	m, err := h.svc.AddMember(c.Request.Context(), me, id, invitee)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toMemberResponse(m))
}

func (h *ConversationHandler) ListMembers(c *gin.Context) {
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
	before, beforeUserID, err := decodeConversationCursor(c.Query("cursor"))
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
	adminsOnly := c.Query("admins_only") == "true"

	list, err := h.svc.ListMembers(c.Request.Context(), me, id, adminsOnly, before, beforeUserID, limit)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	resp := memberListResponse{Members: make([]memberProfileResponse, 0, len(list))}
	for _, mp := range list {
		resp.Members = append(resp.Members, toMemberProfileResponse(mp))
	}
	if eff := usecaseconv.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeConversationCursor(last.JoinedAt, last.UserID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ConversationHandler) RemoveMember(c *gin.Context) {
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
	target, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), me, id, target); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ConversationHandler) SetMemberTier(c *gin.Context) {
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
	target, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req setTierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.SetMemberTier(c.Request.Context(), me, id, target, domainconv.AdminTier(req.AdminTier))
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMemberResponse(m))
}

// SetOwnTier handles PATCH /:id/members/me — the creator stepping down
// (admin_tier: "") or restoring themselves (admin_tier: "creator_admin").
func (h *ConversationHandler) SetOwnTier(c *gin.Context) {
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
	var req setTierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.SetMemberTier(c.Request.Context(), me, id, me, domainconv.AdminTier(req.AdminTier))
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMemberResponse(m))
}

func (h *ConversationHandler) LeaveConversation(c *gin.Context) {
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
	if err := h.svc.LeaveConversation(c.Request.Context(), me, id); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ConversationHandler) Archive(c *gin.Context)   { h.setArchived(c, true) }
func (h *ConversationHandler) Unarchive(c *gin.Context) { h.setArchived(c, false) }

func (h *ConversationHandler) setArchived(c *gin.Context, archived bool) {
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
	if err := h.svc.SetArchived(c.Request.Context(), me, id, archived); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ConversationHandler) ClaimCreator(c *gin.Context) {
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
	m, err := h.svc.ClaimCreator(c.Request.Context(), me, id)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMemberResponse(m))
}
