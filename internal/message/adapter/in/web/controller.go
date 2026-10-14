// internal/message/adapter/in/web/controller.go

// Package web is the message context's primary ("driving") HTTP adapter: it
// translates gin requests into port/in calls and port/in results into the
// same JSON shapes the pre-retrofit internal/adapter/http.MessageHandler
// produced.
package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	domainconv "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/message/application/command"
	"pet-messenger/internal/message/application/port/in"
	"pet-messenger/internal/message/application/query"
	domainmsg "pet-messenger/internal/message/domain"
)

// MessageHandler is the primary adapter for every message route. Each
// field is the one-method use-case interface the matching handler method
// calls.
type MessageHandler struct {
	send     in.SendMessageUseCase
	edit     in.EditMessageUseCase
	del      in.DeleteMessageUseCase
	list     in.ListMessagesUseCase
	markRead in.MarkReadUseCase
}

// NewMessageHandler wires the handler against all five use cases.
func NewMessageHandler(send in.SendMessageUseCase, edit in.EditMessageUseCase, del in.DeleteMessageUseCase, list in.ListMessagesUseCase, markRead in.MarkReadUseCase) *MessageHandler {
	return &MessageHandler{send: send, edit: edit, del: del, list: list, markRead: markRead}
}

// RegisterRoutes mounts the message routes on rg (the /conversations group,
// which already carries AuthMiddleware). Every route is nested under
// /:id/messages so the conversation id is the membership anchor.
func (h *MessageHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/:id/messages", h.Send)
	rg.GET("/:id/messages", h.List)
	rg.PATCH("/:id/messages/:mid", h.Edit)
	rg.DELETE("/:id/messages/:mid", h.Delete)
	rg.POST("/:id/read", h.MarkRead)
}

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported — it is unexported in that package)
// from internal/adapter/http, matching the existing
// internal/conversation/adapter/in/web and internal/user/adapter/in/web
// precedent.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
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
	SystemEvent    *string    `json:"system_event,omitempty"`
	SystemTargetID *string    `json:"system_target_id,omitempty"`
	EditedAt       *time.Time `json:"edited_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadBy         int        `json:"read_by"`
	RecipientCount int        `json:"recipient_count"`
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
		ReadBy:         m.ReadBy,
		RecipientCount: m.RecipientCount,
	}
	if m.SenderID != nil {
		s := m.SenderID.String()
		r.SenderID = &s
	}
	if m.ReplyToID != nil {
		s := m.ReplyToID.String()
		r.ReplyToID = &s
	}
	if m.SystemEvent != "" {
		s := string(m.SystemEvent)
		r.SystemEvent = &s
	}
	if m.SystemTargetID != nil {
		s := m.SystemTargetID.String()
		r.SystemTargetID = &s
	}
	return r
}

type markConversationReadRequest struct {
	MessageID string `json:"message_id" binding:"required"`
}

// MarkRead persists the last message currently visible to the authenticated
// user. The client sends one cursor per newly loaded page, not one request per
// individual message.
func (h *MessageHandler) MarkRead(c *gin.Context) {
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
	var req markConversationReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	messageID, err := uuid.Parse(req.MessageID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	if err := h.markRead.Handle(c.Request.Context(), in.MarkReadCommand{Actor: actor, ConversationID: convID, MessageID: messageID}); err != nil {
		writeMessageError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeMessageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainconv.ErrNotMember),
		errors.Is(err, command.ErrCannotPost),
		errors.Is(err, command.ErrNotAuthor):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, domainmsg.ErrNotFound),
		errors.Is(err, domainmsg.ErrConversationNotFound),
		errors.Is(err, domainconv.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, command.ErrReplyNotFound),
		errors.Is(err, query.ErrConflictingCursors),
		errors.Is(err, domainmsg.ErrEmptyContent),
		errors.Is(err, domainmsg.ErrContentTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domainmsg.ErrNotEditable),
		errors.Is(err, command.ErrConversationArchived):
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

// Send posts a new text message to the route's conversation.
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
	m, err := h.send.Handle(c.Request.Context(), in.SendMessageCommand{Actor: actor, ConversationID: convID, Content: req.Content, ReplyToID: replyTo, IdempotencyKey: req.IdempotencyKey})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.Header("Location", "/conversations/"+convID.String()+"/messages/"+m.ID.String())
	c.JSON(http.StatusCreated, toMessageResponse(m))
}

// List returns a page of the route's conversation history.
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

	msgs, err := h.list.Handle(c.Request.Context(), in.ListMessagesQuery{Actor: actor, ConversationID: convID, AfterSeq: afterSeq, BeforeSeq: beforeSeq, Limit: limit})
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
	if eff := query.PageLimit(limit); len(msgs) == eff && len(msgs) > 0 {
		resp.HasMore = true
		resp.NextSeq = msgs[len(msgs)-1].Seq
	}
	c.JSON(http.StatusOK, resp)
}

// Edit replaces the content of the route's message.
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
	m, err := h.edit.Handle(c.Request.Context(), in.EditMessageCommand{Actor: actor, ConversationID: convID, MessageID: msgID, Content: req.Content})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMessageResponse(m))
}

// Delete soft-deletes the route's message.
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
	m, err := h.del.Handle(c.Request.Context(), in.DeleteMessageCommand{Actor: actor, ConversationID: convID, MessageID: msgID})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMessageResponse(m))
}
