// internal/notification/adapter/in/web/controller_test.go
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/query"
	domainnotif "pet-messenger/internal/notification/domain"
)

type fakeList struct {
	handle func(q in.ListNotificationsQuery) ([]*domainnotif.Notification, error)
}

func (f *fakeList) Handle(_ context.Context, q in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
	return f.handle(q)
}

type fakeUnreadCount struct {
	handle func() (int, error)
}

func (f *fakeUnreadCount) Handle(context.Context, in.UnreadCountQuery) (int, error) {
	return f.handle()
}

type fakeMarkRead struct {
	handle func(cmd in.MarkReadThroughCommand) (int64, error)
}

func (f *fakeMarkRead) Handle(_ context.Context, cmd in.MarkReadThroughCommand) (int64, error) {
	return f.handle(cmd)
}

func notifRouter(h *NotificationHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/notifications")
	if actor != nil {
		grp.Use(func(c *gin.Context) { c.Set(httpadapter.ContextUserIDKey, *actor); c.Next() })
	}
	h.RegisterRoutes(grp)
	return r
}

func doNotif(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
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
	h := NewNotificationHandler(&fakeList{
		handle: func(in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
			t.Fatal("list must not be called without an actor")
			return nil, nil
		},
	}, &fakeUnreadCount{}, &fakeMarkRead{})
	r := notifRouter(h, nil)
	w := doNotif(t, r, http.MethodGet, "/notifications", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestNotificationHandler_List_OKWithNextCursorAndUnread(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	base := time.Now().UTC()
	full := make([]*domainnotif.Notification, query.DefaultPageLimit)
	for i := range full {
		full[i] = sampleNotif(me, base.Add(-time.Duration(i)*time.Minute))
	}
	h := NewNotificationHandler(&fakeList{
		handle: func(q in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
			if q.Actor != me {
				t.Errorf("Actor = %v, want %v", q.Actor, me)
			}
			if q.Limit != 0 {
				t.Errorf("Limit = %d, want 0 (raw, unclamped)", q.Limit)
			}
			return full, nil
		},
	}, &fakeUnreadCount{handle: func() (int, error) { return 5, nil }}, &fakeMarkRead{})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodGet, "/notifications", "")
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
	if len(body.Notifications) != query.DefaultPageLimit {
		t.Errorf("len = %d, want %d", len(body.Notifications), query.DefaultPageLimit)
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
	if row.ReadAt != nil {
		t.Errorf("read_at = %v, want null", row.ReadAt)
	}
}

func TestNotificationHandler_List_ShortPageNoCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewNotificationHandler(&fakeList{
		handle: func(in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
			return []*domainnotif.Notification{sampleNotif(me, time.Now().UTC())}, nil
		},
	}, &fakeUnreadCount{handle: func() (int, error) { return 1, nil }}, &fakeMarkRead{})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodGet, "/notifications", "")
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
	h := NewNotificationHandler(&fakeList{
		handle: func(in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
			t.Fatal("list must not be called on a bad cursor")
			return nil, nil
		},
	}, &fakeUnreadCount{}, &fakeMarkRead{})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodGet, "/notifications?cursor=%21%21%21", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_List_BadLimit(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewNotificationHandler(&fakeList{
		handle: func(in.ListNotificationsQuery) ([]*domainnotif.Notification, error) {
			t.Fatal("list must not be called on a bad limit")
			return nil, nil
		},
	}, &fakeUnreadCount{}, &fakeMarkRead{})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodGet, "/notifications?limit=abc", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_MarkRead_All(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	var gotTS time.Time
	var gotID uuid.UUID
	h := NewNotificationHandler(&fakeList{}, &fakeUnreadCount{}, &fakeMarkRead{
		handle: func(cmd in.MarkReadThroughCommand) (int64, error) {
			gotTS, gotID = cmd.ThroughCreatedAt, cmd.ThroughID
			if cmd.Actor != me {
				t.Errorf("Actor = %v, want %v", cmd.Actor, me)
			}
			return 9, nil
		},
	})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodPost, "/notifications/read", `{}`)
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
	h := NewNotificationHandler(&fakeList{}, &fakeUnreadCount{}, &fakeMarkRead{
		handle: func(cmd in.MarkReadThroughCommand) (int64, error) {
			gotTS, gotID = cmd.ThroughCreatedAt, cmd.ThroughID
			return 2, nil
		},
	})
	r := notifRouter(h, &me)
	cursor := encodeNotificationCursor(wantTS, wantID)
	w := doNotif(t, r, http.MethodPost, "/notifications/read", `{"through_cursor":"`+cursor+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if !gotTS.Equal(wantTS) || gotID != wantID {
		t.Errorf("got (%v, %v), want (%v, %v)", gotTS, gotID, wantTS, wantID)
	}
}

func TestNotificationHandler_MarkRead_BadCursor(t *testing.T) {
	me := uuid.Must(uuid.NewV7())
	h := NewNotificationHandler(&fakeList{}, &fakeUnreadCount{}, &fakeMarkRead{
		handle: func(in.MarkReadThroughCommand) (int64, error) {
			t.Fatal("markRead must not be called on a bad cursor")
			return 0, nil
		},
	})
	r := notifRouter(h, &me)
	w := doNotif(t, r, http.MethodPost, "/notifications/read", `{"through_cursor":"!!!"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNotificationHandler_MarkRead_Unauthenticated(t *testing.T) {
	h := NewNotificationHandler(&fakeList{}, &fakeUnreadCount{}, &fakeMarkRead{
		handle: func(in.MarkReadThroughCommand) (int64, error) {
			t.Fatal("markRead must not be called without an actor")
			return 0, nil
		},
	})
	r := notifRouter(h, nil)
	w := doNotif(t, r, http.MethodPost, "/notifications/read", `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
