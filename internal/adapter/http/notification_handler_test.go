package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainnotif "pet-messenger/internal/domain/notification"
	usecasenotif "pet-messenger/internal/usecase/notification"
)

type fakeNotificationService struct {
	list        func(userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error)
	markRead    func(userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error)
	unreadCount func(userID uuid.UUID) (int, error)
}

func (f *fakeNotificationService) List(_ context.Context, userID uuid.UUID, before time.Time, beforeID uuid.UUID, limit int) ([]*domainnotif.Notification, error) {
	return f.list(userID, before, beforeID, limit)
}
func (f *fakeNotificationService) MarkReadThrough(_ context.Context, userID uuid.UUID, throughCreatedAt time.Time, throughID uuid.UUID) (int64, error) {
	return f.markRead(userID, throughCreatedAt, throughID)
}
func (f *fakeNotificationService) UnreadCount(_ context.Context, userID uuid.UUID) (int, error) {
	return f.unreadCount(userID)
}

func notifRouter(h *NotificationHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/notifications")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func sampleNotif(userID uuid.UUID, createdAt time.Time) *domainnotif.Notification {
	return &domainnotif.Notification{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Kind:      domainnotif.KindMessage,
		Payload:   json.RawMessage(`{"conversation_id":"c1","message_id":"m1"}`),
		CreatedAt: createdAt,
	}
}

func TestNotificationHandler_List_Unauthenticated(t *testing.T) {
	h := &NotificationHandler{svc: &fakeNotificationService{
		list: func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainnotif.Notification, error) {
			t.Fatal("service must not be called without an actor")
			return nil, nil
		},
	}}
	r := notifRouter(h, nil)
	w := doConvJSON(t, r, http.MethodGet, "/notifications", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestNotificationHandler_List_OKWithNextCursorAndUnread(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	base := time.Now().UTC()
	full := make([]*domainnotif.Notification, usecasenotif.DefaultPageLimit)
	for i := range full {
		full[i] = sampleNotif(me, base.Add(-time.Duration(i)*time.Minute))
	}
	h := &NotificationHandler{svc: &fakeNotificationService{
		list: func(userID uuid.UUID, _ time.Time, _ uuid.UUID, limit int) ([]*domainnotif.Notification, error) {
			if userID != me {
				t.Errorf("List userID = %v, want %v", userID, me)
			}
			if limit != 0 {
				t.Errorf("List limit = %d, want 0 (raw, unclamped)", limit)
			}
			return full, nil
		},
		unreadCount: func(uuid.UUID) (int, error) { return 5, nil },
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/notifications", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Notifications []struct {
			ID        string          `json:"id"`
			Kind      string          `json:"kind"`
			Payload   json.RawMessage `json:"payload"`
			CreatedAt time.Time       `json:"created_at"`
			ReadAt    *time.Time      `json:"read_at"`
		} `json:"notifications"`
		NextCursor  string `json:"next_cursor"`
		UnreadCount int    `json:"unread_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Notifications) != usecasenotif.DefaultPageLimit {
		t.Errorf("len = %d, want %d", len(body.Notifications), usecasenotif.DefaultPageLimit)
	}
	if body.NextCursor == "" {
		t.Error("want a non-empty next_cursor on a full page")
	}
	if body.UnreadCount != 5 {
		t.Errorf("unread_count = %d, want 5", body.UnreadCount)
	}
	row := body.Notifications[0]
	if row.ID != full[0].ID.String() || row.Kind != string(domainnotif.KindMessage) {
		t.Errorf("row = %+v", row)
	}
	var payload struct {
		ConversationID string `json:"conversation_id"`
		MessageID      string `json:"message_id"`
	}
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if payload.ConversationID != "c1" || payload.MessageID != "m1" {
		t.Errorf("payload = %+v", payload)
	}
	if row.ReadAt != nil {
		t.Errorf("read_at = %v, want null", row.ReadAt)
	}
}

func TestNotificationHandler_List_ShortPageNoCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &NotificationHandler{svc: &fakeNotificationService{
		list: func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainnotif.Notification, error) {
			return []*domainnotif.Notification{sampleNotif(me, time.Now().UTC())}, nil
		},
		unreadCount: func(uuid.UUID) (int, error) { return 1, nil },
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/notifications", "")
	var body struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty on a short page", body.NextCursor)
	}
}

func TestNotificationHandler_List_BadCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &NotificationHandler{svc: &fakeNotificationService{
		list: func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainnotif.Notification, error) {
			t.Fatal("service must not be called on a bad cursor")
			return nil, nil
		},
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/notifications?cursor=%21%21%21", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_List_BadLimit(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &NotificationHandler{svc: &fakeNotificationService{
		list: func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainnotif.Notification, error) {
			t.Fatal("service must not be called on a bad limit")
			return nil, nil
		},
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/notifications?limit=abc", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_List_ClampedLimitStillPaginates(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	// A caller asking for limit=5 gets DefaultPageLimit rows from the (fake)
	// service; next_cursor must still be emitted because len == PageLimit(5).
	base := time.Now().UTC()
	rows := make([]*domainnotif.Notification, usecasenotif.PageLimit(5))
	for i := range rows {
		rows[i] = sampleNotif(me, base.Add(-time.Duration(i)*time.Minute))
	}
	h := &NotificationHandler{svc: &fakeNotificationService{
		list:        func(uuid.UUID, time.Time, uuid.UUID, int) ([]*domainnotif.Notification, error) { return rows, nil },
		unreadCount: func(uuid.UUID) (int, error) { return 0, nil },
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodGet, "/notifications?limit=5", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.NextCursor == "" {
		t.Error("want next_cursor when the page fills the clamped limit")
	}
}

func TestNotificationHandler_MarkRead_All(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	var gotTS time.Time
	var gotID uuid.UUID
	h := &NotificationHandler{svc: &fakeNotificationService{
		markRead: func(userID uuid.UUID, ts time.Time, id uuid.UUID) (int64, error) {
			gotTS, gotID = ts, id
			if userID != me {
				t.Errorf("MarkReadThrough userID = %v, want %v", userID, me)
			}
			return 9, nil
		},
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/notifications/read", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Marked int64 `json:"marked"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Marked != 9 {
		t.Errorf("marked = %d, want 9", body.Marked)
	}
	if !gotTS.IsZero() || gotID != uuid.Nil {
		t.Errorf("empty cursor should mark all; got (%v, %v)", gotTS, gotID)
	}
}

func TestNotificationHandler_MarkRead_ThroughCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	wantTS := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	wantID := uuid.Must(uuid.NewV7())
	var gotTS time.Time
	var gotID uuid.UUID
	h := &NotificationHandler{svc: &fakeNotificationService{
		markRead: func(_ uuid.UUID, ts time.Time, id uuid.UUID) (int64, error) {
			gotTS, gotID = ts, id
			return 2, nil
		},
	}}
	r := notifRouter(h, &me)
	cursor := encodeNotificationCursor(wantTS, wantID)
	w := doConvJSON(t, r, http.MethodPost, "/notifications/read", `{"through_cursor":"`+cursor+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if !gotTS.Equal(wantTS) || gotID != wantID {
		t.Errorf("service got (%v, %v), want (%v, %v)", gotTS, gotID, wantTS, wantID)
	}
}

func TestNotificationHandler_MarkRead_BadCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := &NotificationHandler{svc: &fakeNotificationService{
		markRead: func(uuid.UUID, time.Time, uuid.UUID) (int64, error) {
			t.Fatal("service must not be called on a bad cursor")
			return 0, nil
		},
	}}
	r := notifRouter(h, &me)
	w := doConvJSON(t, r, http.MethodPost, "/notifications/read", `{"through_cursor":"!!!"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_MarkRead_Unauthenticated(t *testing.T) {
	h := &NotificationHandler{svc: &fakeNotificationService{
		markRead: func(uuid.UUID, time.Time, uuid.UUID) (int64, error) {
			t.Fatal("service must not be called without an actor")
			return 0, nil
		},
	}}
	r := notifRouter(h, nil)
	w := doConvJSON(t, r, http.MethodPost, "/notifications/read", `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
