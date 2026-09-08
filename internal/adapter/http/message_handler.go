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
	domainmsg "pet-messenger/internal/domain/message"
	usecasemsg "pet-messenger/internal/usecase/message"
)

// messageService is satisfied by *usecasemsg.Service; declared here so tests can
// inject a fake without a database.
type messageService interface {
	SendMessage(ctx context.Context, actor, convID uuid.UUID, content string, replyToID *uuid.UUID, idempotencyKey string) (*domainmsg.Message, error)
	EditMessage(ctx context.Context, actor, convID, messageID uuid.UUID, newContent string) (*domainmsg.Message, error)
	DeleteMessage(ctx context.Context, actor, convID, messageID uuid.UUID) (*domainmsg.Message, error)
	ListMessages(ctx context.Context, actor, convID uuid.UUID, afterSeq, beforeSeq *int64, limit int) ([]*domainmsg.Message, error)
}

type MessageHandler struct {
	svc messageService
}

func NewMessageHandler(svc *usecasemsg.Service) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// RegisterRoutes mounts the message routes on rg (the /conversations group,
// which already carries AuthMiddleware). Every route is nested under
// /:id/messages so the conversation id is the membership anchor.
func (h *MessageHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/messages", h.Send)
	rg.GET("/:id/messages", h.List)
	rg.PATCH("/:id/messages/:mid", h.Edit)
	rg.DELETE("/:id/messages/:mid", h.Delete)
}

type sendMessageRequest struct {
	Content        string `json:"content" binding:"required"`
	ReplyToID      string `json:"reply_to_id"`
	IdempotencyKey string `json:"idempotency_key" binding:"max=255"`
}

type editMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type messageResponse struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	Seq            int64      `json:"seq"`
	SenderID       *string    `json:"sender_id"`
	Type           string     `json:"type"`
	Content        string     `json:"content"`
	ReplyToID      *string    `json:"reply_to_id"`
	EditedAt       *time.Time `json:"edited_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type messageListResponse struct {
	Messages []messageResponse `json:"messages"`
	NextSeq  int64             `json:"next_seq,omitempty"`
	HasMore  bool              `json:"has_more"`
}

func toMessageResponse(m *domainmsg.Message) messageResponse {
	r := messageResponse{
		ID:             m.ID.String(),
		ConversationID: m.ConversationID.String(),
		Seq:            m.Seq,
		Type:           string(m.Type),
		Content:        m.Content,
		EditedAt:       m.EditedAt,
		DeletedAt:      m.DeletedAt,
		CreatedAt:      m.CreatedAt,
	}
	if m.SenderID != nil {
		s := m.SenderID.String()
		r.SenderID = &s
	}
	if m.ReplyToID != nil {
		s := m.ReplyToID.String()
		r.ReplyToID = &s
	}
	return r
}

func writeMessageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainconv.ErrNotMember),
		errors.Is(err, usecasemsg.ErrCannotPost),
		errors.Is(err, usecasemsg.ErrNotAuthor):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, domainmsg.ErrNotFound),
		errors.Is(err, domainmsg.ErrConversationNotFound),
		errors.Is(err, domainconv.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, usecasemsg.ErrReplyNotFound),
		errors.Is(err, usecasemsg.ErrConflictingCursors),
		errors.Is(err, domainmsg.ErrEmptyContent),
		errors.Is(err, domainmsg.ErrContentTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domainmsg.ErrNotEditable):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// parseSeqQuery returns (nil, nil) when the param is absent, (&n, nil) when it
// is a valid int64, or (nil, err) when it is present but unparseable.
func parseSeqQuery(c *gin.Context, name string) (*int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (h *MessageHandler) Send(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var replyTo *uuid.UUID
	if req.ReplyToID != "" {
		id, err := uuid.Parse(req.ReplyToID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reply_to_id"})
			return
		}
		replyTo = &id
	}
	m, err := h.svc.SendMessage(c.Request.Context(), actor, convID, req.Content, replyTo, req.IdempotencyKey)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.Header("Location", "/conversations/"+convID.String()+"/messages/"+m.ID.String())
	c.JSON(http.StatusCreated, toMessageResponse(m))
}

func (h *MessageHandler) List(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	afterSeq, err := parseSeqQuery(c, "after_seq")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_seq"})
		return
	}
	beforeSeq, err := parseSeqQuery(c, "before_seq")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_seq"})
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

	msgs, err := h.svc.ListMessages(c.Request.Context(), actor, convID, afterSeq, beforeSeq, limit)
	if err != nil {
		writeMessageError(c, err)
		return
	}

	resp := messageListResponse{Messages: make([]messageResponse, 0, len(msgs))}
	for _, m := range msgs {
		resp.Messages = append(resp.Messages, toMessageResponse(m))
	}
	// A page that is exactly the effective limit may have more behind it; hand
	// back the far-edge seq (min for a backward page, max for a forward page —
	// in both cases the last element) so the client re-issues the same
	// direction param with it.
	if eff := usecasemsg.PageLimit(limit); len(msgs) == eff && len(msgs) > 0 {
		resp.HasMore = true
		resp.NextSeq = msgs[len(msgs)-1].Seq
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) Edit(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	msgID, err := uuid.Parse(c.Param("mid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	var req editMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.EditMessage(c.Request.Context(), actor, convID, msgID, req.Content)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMessageResponse(m))
}

func (h *MessageHandler) Delete(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	msgID, err := uuid.Parse(c.Param("mid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	m, err := h.svc.DeleteMessage(c.Request.Context(), actor, convID, msgID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMessageResponse(m))
}
