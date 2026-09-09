package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/domain/notification"
	usecasenotif "pet-messenger/internal/usecase/notification"
)

// notificationService is satisfied by *usecasenotif.Service; declared here so
// tests can inject a fake without a database.
type notificationService interface {
	List(ctx context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error)
	MarkReadThrough(ctx context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error)
	UnreadCount(ctx context.Context, userID uuid.UUID) (int, error)
}

type NotificationHandler struct {
	svc notificationService
}

func NewNotificationHandler(svc *usecasenotif.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// RegisterRoutes mounts the notification routes on rg. rg is expected to already
// carry AuthMiddleware, so every handler can rely on ContextUserIDKey.
func (h *NotificationHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("/read", h.MarkRead)
}

type notificationResponse struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
}

type notificationListResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	NextCursor    string                 `json:"next_cursor,omitempty"`
	UnreadCount   int                    `json:"unread_count"`
}

func toNotificationResponse(n *domainnotif.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID.String(),
		Kind:      string(n.Kind),
		Payload:   n.Payload,
		CreatedAt: n.CreatedAt,
		ReadAt:    n.ReadAt,
	}
}

// markReadRequest.ThroughCursor is intentionally not `binding:"required"` — an
// absent or empty cursor means "mark every unread notification read".
type markReadRequest struct {
	ThroughCursor string `json:"through_cursor"`
}

func (h *NotificationHandler) List(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	before, beforeID, err := decodeNotificationCursor(c.Query("cursor"))
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

	list, err := h.svc.List(c.Request.Context(), me, before, beforeID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	unread, err := h.svc.UnreadCount(c.Request.Context(), me)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	resp := notificationListResponse{
		Notifications: make([]notificationResponse, 0, len(list)),
		UnreadCount:   unread,
	}
	for _, n := range list {
		resp.Notifications = append(resp.Notifications, toNotificationResponse(n))
	}
	// A page that is exactly the effective limit may have more behind it; hand
	// back the keyset of the last row so the client can ask for the next page.
	if eff := usecasenotif.PageLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeNotificationCursor(last.CreatedAt, last.ID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	me, ok := actorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		through   time.Time
		throughID uuid.UUID
	)
	if req.ThroughCursor != "" {
		ts, id, err := decodeNotificationCursor(req.ThroughCursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
			return
		}
		through, throughID = ts, id
	}

	marked, err := h.svc.MarkReadThrough(c.Request.Context(), me, through, throughID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"marked": marked})
}
