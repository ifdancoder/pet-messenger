// Package web is the conversation context's primary ("driving") HTTP
// adapter: it translates gin requests into port/in calls and port/in
// results into the same JSON shapes the pre-retrofit
// internal/adapter/http.ConversationHandler produced.
package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	"pet-messenger/internal/conversation/application"
	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/in"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/domain"
)

// ConversationHandler is the primary adapter for every /conversations
// route. Each field is the one-method use-case interface the matching
// handler method calls: no dependency wider than what that one route
// needs.
type ConversationHandler struct {
	openDirect         in.OpenDirectUseCase
	createGroupChat    in.CreateGroupChatUseCase
	createChannel      in.CreateChannelUseCase
	joinChannel        in.JoinChannelUseCase
	leaveConversation  in.LeaveConversationUseCase
	addMember          in.AddMemberUseCase
	removeMember       in.RemoveMemberUseCase
	setMemberTier      in.SetMemberTierUseCase
	claimCreator       in.ClaimCreatorUseCase
	setArchived        in.SetArchivedUseCase
	listMine           in.ListMineUseCase
	listPublicChannels in.ListPublicChannelsUseCase
	listMembers        in.ListMembersUseCase
}

// NewConversationHandler wires the handler against all 13 use cases.
func NewConversationHandler(
	openDirect in.OpenDirectUseCase,
	createGroupChat in.CreateGroupChatUseCase,
	createChannel in.CreateChannelUseCase,
	joinChannel in.JoinChannelUseCase,
	leaveConversation in.LeaveConversationUseCase,
	addMember in.AddMemberUseCase,
	removeMember in.RemoveMemberUseCase,
	setMemberTier in.SetMemberTierUseCase,
	claimCreator in.ClaimCreatorUseCase,
	setArchived in.SetArchivedUseCase,
	listMine in.ListMineUseCase,
	listPublicChannels in.ListPublicChannelsUseCase,
	listMembers in.ListMembersUseCase,
) *ConversationHandler {
	return &ConversationHandler{
		openDirect: openDirect, createGroupChat: createGroupChat, createChannel: createChannel,
		joinChannel: joinChannel, leaveConversation: leaveConversation, addMember: addMember,
		removeMember: removeMember, setMemberTier: setMemberTier, claimCreator: claimCreator,
		setArchived: setArchived, listMine: listMine, listPublicChannels: listPublicChannels,
		listMembers: listMembers,
	}
}

// RegisterRoutes mounts the conversation routes on rg. rg is expected to
// already carry AuthMiddleware, so every handler can rely on
// httpadapter.ContextUserIDKey.
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

func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
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

// setTierRequest.AdminTier is intentionally not `binding:"required"`: the
// empty string is a valid value meaning "no tier".
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

func toMemberResponse(m *domain.Member) memberResponse {
	return memberResponse{UserID: m.UserID.String(), Role: string(m.Role), AdminTier: string(m.AdminTier), IsCreator: m.IsCreator, JoinedAt: m.JoinedAt}
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

func toMemberProfileResponse(mp *domain.MemberProfile) memberProfileResponse {
	return memberProfileResponse{memberResponse: toMemberResponse(&mp.Member), Username: mp.Username, DisplayName: mp.DisplayName, AvatarURL: mp.AvatarURL}
}

type conversationResponse struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	Title             string     `json:"title"`
	AvatarURL         string     `json:"avatar_url"`
	Public            bool       `json:"public"`
	CreatedAt         time.Time  `json:"created_at"`
	LastMessageAt     time.Time  `json:"last_message_at"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	DirectUsername    string     `json:"direct_username,omitempty"`
	UnreadCount       int        `json:"unread_count"`
	DirectDisplayName string     `json:"direct_display_name,omitempty"`
	DirectAvatarURL   string     `json:"direct_avatar_url,omitempty"`
}

type conversationListResponse struct {
	Conversations []conversationResponse `json:"conversations"`
	NextCursor    string                 `json:"next_cursor,omitempty"`
}

type channelListResponse struct {
	Channels   []conversationResponse `json:"channels"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func toConversationResponse(c *domain.Conversation) conversationResponse {
	return conversationResponse{
		ID: c.ID.String(), Kind: string(c.Kind), Title: c.Title, AvatarURL: c.AvatarURL, Public: c.Public,
		CreatedAt: c.CreatedAt, LastMessageAt: c.LastMessageAt, ArchivedAt: c.ArchivedAt,
		DirectUsername: c.DirectUsername, UnreadCount: c.UnreadCount,
		DirectDisplayName: c.DirectDisplayName, DirectAvatarURL: c.DirectAvatarURL,
	}
}

// toConversationResponseFromInbox maps a ListMine row (out.InboxEntry) onto
// the same conversationResponse shape as every other conversation-list
// endpoint.
func toConversationResponseFromInbox(e *out.InboxEntry) conversationResponse {
	return conversationResponse{
		ID: e.ConversationID.String(), Kind: string(e.Kind), Title: e.Title, AvatarURL: e.AvatarURL, Public: e.Public,
		CreatedAt: e.CreatedAt, LastMessageAt: e.LastMessageAt, ArchivedAt: e.ArchivedAt,
		DirectUsername: e.DirectUsername, UnreadCount: e.UnreadCount,
		DirectDisplayName: e.DirectDisplayName, DirectAvatarURL: e.DirectAvatarURL,
	}
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, command.ErrCannotDirectSelf),
		errors.Is(err, domain.ErrInvalidDirectMembers),
		errors.Is(err, domain.ErrInvalidTitle),
		errors.Is(err, domain.ErrInvalidCreator),
		errors.Is(err, domain.ErrInvalidKind),
		errors.Is(err, command.ErrCannotKickSelf),
		errors.Is(err, command.ErrInvalidTier):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrConversationArchived),
		errors.Is(err, domain.ErrCreatorSeatTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, command.ErrForbidden),
		errors.Is(err, domain.ErrNotCreatorHeir):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, command.ErrRecipientNotFound),
		errors.Is(err, command.ErrInviteeNotFound),
		errors.Is(err, domain.ErrNotChannel),
		errors.Is(err, domain.ErrNotFound),
		errors.Is(err, domain.ErrNotMember):
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
	conv, err := h.openDirect.Handle(c.Request.Context(), in.OpenDirectCommand{Me: me, Other: other})
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
	conv, err := h.createGroupChat.Handle(c.Request.Context(), in.CreateGroupChatCommand{Creator: me, Title: req.Title})
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

	list, err := h.listMine.Handle(c.Request.Context(), in.ListMineQuery{UserID: me, Before: before, BeforeID: beforeID, Limit: limit})
	if err != nil {
		writeConversationError(c, err)
		return
	}

	resp := conversationListResponse{Conversations: make([]conversationResponse, 0, len(list))}
	for _, e := range list {
		resp.Conversations = append(resp.Conversations, toConversationResponseFromInbox(e))
	}
	if eff := application.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeConversationCursor(last.LastMessageAt, last.ConversationID)
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
	conv, err := h.createChannel.Handle(c.Request.Context(), in.CreateChannelCommand{Creator: me, Kind: domain.Kind(req.Kind), Title: req.Title, Public: req.Public})
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
	q := c.Query("q")
	if len(q) > 100 {
		q = q[:100]
	}

	list, err := h.listPublicChannels.Handle(c.Request.Context(), in.ListPublicChannelsQuery{Query: q, Before: before, BeforeID: beforeID, Limit: limit})
	if err != nil {
		writeConversationError(c, err)
		return
	}

	resp := channelListResponse{Channels: make([]conversationResponse, 0, len(list))}
	for _, cv := range list {
		resp.Channels = append(resp.Channels, toConversationResponse(cv))
	}
	if eff := application.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
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
	conv, err := h.joinChannel.Handle(c.Request.Context(), in.JoinChannelCommand{Actor: me, ChannelID: id})
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
	m, err := h.addMember.Handle(c.Request.Context(), in.AddMemberCommand{Actor: me, ConversationID: id, Invitee: invitee})
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

	list, err := h.listMembers.Handle(c.Request.Context(), in.ListMembersQuery{Actor: me, ConversationID: id, AdminsOnly: adminsOnly, Before: before, BeforeUserID: beforeUserID, Limit: limit})
	if err != nil {
		writeConversationError(c, err)
		return
	}
	resp := memberListResponse{Members: make([]memberProfileResponse, 0, len(list))}
	for _, mp := range list {
		resp.Members = append(resp.Members, toMemberProfileResponse(mp))
	}
	if eff := application.EffectiveListLimit(limit); len(list) == eff && len(list) > 0 {
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
	if err := h.removeMember.Handle(c.Request.Context(), in.RemoveMemberCommand{Actor: me, ConversationID: id, Target: target}); err != nil {
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
	m, err := h.setMemberTier.Handle(c.Request.Context(), in.SetMemberTierCommand{Actor: me, ConversationID: id, Target: target, Tier: domain.AdminTier(req.AdminTier)})
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMemberResponse(m))
}

// SetOwnTier handles PATCH /:id/members/me: the creator stepping down
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
	m, err := h.setMemberTier.Handle(c.Request.Context(), in.SetMemberTierCommand{Actor: me, ConversationID: id, Target: me, Tier: domain.AdminTier(req.AdminTier)})
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
	if err := h.leaveConversation.Handle(c.Request.Context(), in.LeaveConversationCommand{Actor: me, ConversationID: id}); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ConversationHandler) Archive(c *gin.Context)   { h.setArchivedRoute(c, true) }
func (h *ConversationHandler) Unarchive(c *gin.Context) { h.setArchivedRoute(c, false) }

func (h *ConversationHandler) setArchivedRoute(c *gin.Context, archived bool) {
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
	if err := h.setArchived.Handle(c.Request.Context(), in.SetArchivedCommand{Actor: me, ConversationID: id, Archived: archived}); err != nil {
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
	m, err := h.claimCreator.Handle(c.Request.Context(), in.ClaimCreatorCommand{Actor: me, ConversationID: id})
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMemberResponse(m))
}
