// Package web is the call context's primary ("driving") HTTP adapter.
package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	callcommand "pet-messenger/internal/call/application/command"
	in "pet-messenger/internal/call/application/port/in"
	out "pet-messenger/internal/call/application/port/out"
	domain "pet-messenger/internal/call/domain"
	conversation "pet-messenger/internal/conversation/domain"
)

// CallHandler is the primary adapter for every call route. Each field is
// the one-method use-case interface the matching handler method calls.
type CallHandler struct {
	start  in.StartCallUseCase
	join   in.JoinCallUseCase
	end    in.EndCallUseCase
	mute   in.MuteParticipantUseCase
	unmute in.UnmuteParticipantUseCase
	remove in.RemoveParticipantUseCase
	live   in.GetLiveForConversationUseCase
}

// NewCallHandler wires the handler against all 7 use cases.
func NewCallHandler(start in.StartCallUseCase, join in.JoinCallUseCase, end in.EndCallUseCase, mute in.MuteParticipantUseCase, unmute in.UnmuteParticipantUseCase, remove in.RemoveParticipantUseCase, live in.GetLiveForConversationUseCase) *CallHandler {
	return &CallHandler{start: start, join: join, end: end, mute: mute, unmute: unmute, remove: remove, live: live}
}

// RegisterConversationRoutes mounts the two conversation-scoped call
// routes on rg (the /conversations group, which already carries
// AuthMiddleware).
func (h *CallHandler) RegisterConversationRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/calls", h.Start)
	rg.GET("/:id/calls/live", h.Live)
}

// RegisterRoutes mounts the five call-scoped routes on rg (the /calls
// group).
func (h *CallHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/join", h.Join)
	rg.POST("/:id/end", h.End)
	rg.POST("/:id/participants/:userId/mute", h.Mute)
	rg.POST("/:id/participants/:userId/unmute", h.Unmute)
	rg.DELETE("/:id/participants/:userId", h.Remove)
}

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported -- it is unexported in that package)
// from internal/adapter/http, matching the established precedent in every
// other context's own controller. A missing actor is not written as a
// response here (bare return) -- matching the original handler exactly:
// AuthMiddleware already rejects the request before this handler runs, so
// this branch has no real route to exercise in production.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// callDTO is the client-visible JSON shape of a call.
type callDTO struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	InitiatorID    string     `json:"initiator_id"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
}

// toCallDTO converts a domain Call into its client-visible JSON shape.
func toCallDTO(c *domain.Call) callDTO {
	return callDTO{
		ID: c.ID.String(), ConversationID: c.ConversationID.String(),
		InitiatorID: c.InitiatorID.String(), Status: string(c.Status),
		StartedAt: c.StartedAt, EndedAt: c.EndedAt,
	}
}

// participantDTO is the client-visible JSON shape of one call participant.
type participantDTO struct {
	UserID     string   `json:"user_id"`
	Role       string   `json:"role"`
	Muted      bool     `json:"muted"`
	SelfMuted  bool     `json:"self_muted"`
	TrackKinds []string `json:"track_kinds"`
	Connected  bool     `json:"connected"`
	Accepted   bool     `json:"accepted"`
}

// toParticipantDTOs converts the out port's participant roster into its
// client-visible JSON shape.
func toParticipantDTOs(participants []out.ParticipantInfo) []participantDTO {
	dtos := make([]participantDTO, len(participants))
	for i, p := range participants {
		dtos[i] = participantDTO{UserID: p.UserID, Role: p.Role, Muted: p.Muted, SelfMuted: p.SelfMuted, TrackKinds: p.TrackKinds, Connected: p.Connected, Accepted: p.Accepted}
	}
	return dtos
}

// Start starts a new call for the route's conversation, or joins its
// already-live call.
// Start godoc
//
//	@Summary		Start (or join) a conversation's call
//	@Description	Starts a new call for the route's conversation, or joins its already-live call.
//	@Tags			calls
//	@Produce		json
//	@Param			id	path		string	true	"Conversation ID"
//	@Success		201	{object}	map[string]interface{}	"call and join payload"
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string	"unauthenticated"
//	@Failure		403	{object}	map[string]string	"not a conversation member"
//	@Failure		404	{object}	map[string]string	"conversation not found"
//	@Failure		409	{object}	map[string]string	"call not supported, or conversation archived"
//	@Failure		503	{object}	map[string]string	"no media nodes available"
//	@Security		BearerAuth
//	@Router			/conversations/{id}/calls [post]
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
	call, join, err := h.start.Handle(c.Request.Context(), in.StartCallCommand{Actor: actor, ConversationID: convID})
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"call": toCallDTO(call), "join": join})
}

// Live returns the route's conversation's live call and current
// participant roster.
// Live godoc
//
//	@Summary		Get a conversation's live call
//	@Description	Returns the route's conversation's live call and current participant roster.
//	@Tags			calls
//	@Produce		json
//	@Param			id	path		string					true	"Conversation ID"
//	@Success		200	{object}	map[string]interface{}	"call and participants"
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string	"unauthenticated"
//	@Failure		403	{object}	map[string]string	"not a conversation member"
//	@Failure		404	{object}	map[string]string	"conversation or live call not found"
//	@Security		BearerAuth
//	@Router			/conversations/{id}/calls/live [get]
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
	call, participants, err := h.live.Handle(c.Request.Context(), in.GetLiveForConversationQuery{Actor: actor, ConversationID: convID})
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"call": toCallDTO(call), "participants": toParticipantDTOs(participants)})
}

// joinRequest is the optional request body for Join.
type joinRequest struct {
	Role string `json:"role"`
}

// Join joins the route's already-live call. An invalid or missing
// requested role falls back to RolePublisher; the request body itself is
// optional.
// Join godoc
//
//	@Summary		Join a live call
//	@Description	Joins the route's already-live call. An invalid or missing requested role falls back to "publisher"; the request body itself is optional.
//	@Tags			calls
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Call ID"
//	@Param			request	body		joinRequest				false	"Requested role"
//	@Success		200		{object}	map[string]interface{}	"join payload"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"not a conversation member"
//	@Failure		404		{object}	map[string]string	"call not found or not live"
//	@Failure		503		{object}	map[string]string	"media unavailable"
//	@Security		BearerAuth
//	@Router			/calls/{id}/join [post]
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
	role := domain.Role(body.Role)
	if !role.Valid() {
		role = domain.RolePublisher
	}
	join, err := h.join.Handle(c.Request.Context(), in.JoinCallCommand{Actor: actor, CallID: callID, Role: role})
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"join": join})
}

// End godoc
//
//	@Summary		End a call
//	@Description	Ends the route's call. Ending an already-ended call is a no-op 204, not an error.
//	@Tags			calls
//	@Param			id	path	string	true	"Call ID"
//	@Success		204	"no body"
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string	"unauthenticated"
//	@Failure		403	{object}	map[string]string	"not permitted to end this call"
//	@Failure		404	{object}	map[string]string	"call not found"
//	@Security		BearerAuth
//	@Router			/calls/{id}/end [post]
func (h *CallHandler) End(c *gin.Context) {
	h.mutate(c, func(ctx context.Context, actor, callID uuid.UUID) error {
		return h.end.Handle(ctx, in.EndCallCommand{Actor: actor, CallID: callID})
	})
}

// Mute godoc
//
//	@Summary		Mute a call participant
//	@Tags			calls
//	@Param			id		path	string	true	"Call ID"
//	@Param			userId	path	string	true	"Target participant's user ID"
//	@Success		204		"no body"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"not permitted, or cannot target self"
//	@Failure		404		{object}	map[string]string	"call not found"
//	@Security		BearerAuth
//	@Router			/calls/{id}/participants/{userId}/mute [post]
func (h *CallHandler) Mute(c *gin.Context) {
	h.targeted(c, func(ctx context.Context, actor, callID, target uuid.UUID) error {
		return h.mute.Handle(ctx, in.MuteParticipantCommand{Actor: actor, CallID: callID, TargetID: target})
	})
}

// Unmute godoc
//
//	@Summary		Unmute a call participant
//	@Tags			calls
//	@Param			id		path	string	true	"Call ID"
//	@Param			userId	path	string	true	"Target participant's user ID"
//	@Success		204		"no body"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"not permitted, or cannot target self"
//	@Failure		404		{object}	map[string]string	"call not found"
//	@Security		BearerAuth
//	@Router			/calls/{id}/participants/{userId}/unmute [post]
func (h *CallHandler) Unmute(c *gin.Context) {
	h.targeted(c, func(ctx context.Context, actor, callID, target uuid.UUID) error {
		return h.unmute.Handle(ctx, in.UnmuteParticipantCommand{Actor: actor, CallID: callID, TargetID: target})
	})
}

// Remove godoc
//
//	@Summary		Remove a call participant
//	@Tags			calls
//	@Param			id		path	string	true	"Call ID"
//	@Param			userId	path	string	true	"Target participant's user ID"
//	@Success		204		"no body"
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string	"unauthenticated"
//	@Failure		403		{object}	map[string]string	"not permitted, or cannot target self"
//	@Failure		404		{object}	map[string]string	"call not found"
//	@Security		BearerAuth
//	@Router			/calls/{id}/participants/{userId} [delete]
func (h *CallHandler) Remove(c *gin.Context) {
	h.targeted(c, func(ctx context.Context, actor, callID, target uuid.UUID) error {
		return h.remove.Handle(ctx, in.RemoveParticipantCommand{Actor: actor, CallID: callID, TargetID: target})
	})
}

// mutate runs fn for a call-scoped mutation that reports success as 204,
// special-casing domain.ErrAlreadyEnded as a no-op 204 rather than an
// error response.
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
		if errors.Is(err, domain.ErrAlreadyEnded) {
			c.Status(http.StatusNoContent)
			return
		}
		writeCallError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// targeted runs fn for a participant-targeted mutation that reports
// success as 204.
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

// writeCallError maps a use-case error to its client-visible status code
// and body. Every sentinel handled here already carries its final
// client-visible "call: ..." (or "conversation: ...") text, so err.Error()
// is used directly instead of a static message.
func writeCallError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, callcommand.ErrCallNotSupported), errors.Is(err, callcommand.ErrModerationNotSupported), errors.Is(err, conversation.ErrConversationArchived), errors.Is(err, callcommand.ErrCannotTargetSelf):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, callcommand.ErrForbidden), errors.Is(err, conversation.ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrCallNotLive), errors.Is(err, conversation.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, callcommand.ErrNoMediaNodes), errors.Is(err, callcommand.ErrMediaUnavailable), errors.Is(err, callcommand.ErrMediaNodeGone):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrAlreadyEnded):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
