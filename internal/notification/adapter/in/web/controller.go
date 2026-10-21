// internal/notification/adapter/in/web/controller.go

// Package web is the notification context's primary ("driving") HTTP
// adapter: it translates gin requests into port/in calls and port/in
// results into the same JSON shapes the pre-retrofit
// internal/adapter/http.NotificationHandler produced.
package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/platform/adapter/http"
	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/query"
	domainnotif "pet-messenger/internal/notification/domain"
)

// actorID reads the authenticated user id AuthMiddleware stored on the
// context. Duplicated (not imported) from internal/adapter/http; it is
// unexported there, matching the precedent every other context's own
// controller already follows.
func actorID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(httpadapter.ContextUserIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

var errInvalidCursor = errors.New("notification: invalid pagination cursor")

// encodeNotificationCursor packs a (created_at, id) keyset position into an
// opaque base64 token for the notification-list API.
func encodeNotificationCursor(t time.Time, id uuid.UUID) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeNotificationCursor is the inverse. An empty string is the "first
// page" sentinel and returns the zero time and uuid.Nil with no error;
// anything that does not parse returns errInvalidCursor.
func decodeNotificationCursor(raw string) (time.Time, uuid.UUID, error) {
	if raw == "" {
		return time.Time{}, uuid.Nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, uuid.Nil, errInvalidCursor
	}
	tsPart, idPart, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.Nil, errInvalidCursor
	}
	ts, err := time.Parse(time.RFC3339Nano, tsPart)
	if err != nil {
		return time.Time{}, uuid.Nil, errInvalidCursor
	}
	id, err := uuid.Parse(idPart)
	if err != nil {
		return time.Time{}, uuid.Nil, errInvalidCursor
	}
	return ts, id, nil
}

// NotificationHandler is the primary HTTP adapter for the notification
// context.
type NotificationHandler struct {
	list        in.ListNotificationsUseCase
	unreadCount in.UnreadCountUseCase
	markRead    in.MarkReadThroughUseCase
}

// NewNotificationHandler wires the notification HTTP routes to their use
// cases.
func NewNotificationHandler(list in.ListNotificationsUseCase, unreadCount in.UnreadCountUseCase, markRead in.MarkReadThroughUseCase) *NotificationHandler {
	return &NotificationHandler{list: list, unreadCount: unreadCount, markRead: markRead}
}

// RegisterRoutes mounts the notification routes on rg. rg is expected to
// already carry AuthMiddleware, so every handler can rely on
// ContextUserIDKey.
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

// markReadRequest.ThroughCursor is intentionally not `binding:"required"`:
// an absent or empty cursor means "mark every unread notification read".
type markReadRequest struct {
	ThroughCursor string `json:"through_cursor"`
}

// List returns a page of the authenticated user's notifications, newest
// first, along with their current unread count.
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

	list, err := h.list.Handle(c.Request.Context(), in.ListNotificationsQuery{Actor: me, Before: before, BeforeID: beforeID, Limit: limit})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	unread, err := h.unreadCount.Handle(c.Request.Context(), in.UnreadCountQuery{Actor: me})
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
	// A page that is exactly the effective limit may have more behind it;
	// hand back the keyset of the last row so the client can ask for the
	// next page.
	if eff := query.PageLimit(limit); len(list) == eff && len(list) > 0 {
		last := list[len(list)-1]
		resp.NextCursor = encodeNotificationCursor(last.CreatedAt, last.ID)
	}
	c.JSON(http.StatusOK, resp)
}

// MarkRead marks every notification of the authenticated user up through
// ThroughCursor as read; an absent or empty cursor marks everything read.
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

	marked, err := h.markRead.Handle(c.Request.Context(), in.MarkReadThroughCommand{Actor: me, ThroughCreatedAt: through, ThroughID: throughID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"marked": marked})
}
