package notification_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/notification"
)

func TestKind_Valid(t *testing.T) {
	for _, k := range []notification.Kind{notification.KindMessage, notification.KindMemberRemoved, notification.KindMemberTierChanged} {
		if !k.Valid() {
			t.Errorf("%q should be Valid", k)
		}
	}
	if notification.Kind("reaction").Valid() {
		t.Error(`Kind("reaction") should be invalid`)
	}
	if !notification.KindCallStarted.Valid() {
		t.Error("KindCallStarted should be Valid")
	}
}

func TestNew(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	payload := json.RawMessage(`{"conversation_id":"x"}`)

	n, err := notification.New(uid, notification.KindMessage, payload, "msg:abc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if n.UserID != uid || n.Kind != notification.KindMessage || n.DedupKey != "msg:abc" {
		t.Fatalf("n = %+v", n)
	}
	if n.ID.Version() != 7 || n.CreatedAt.IsZero() || n.ReadAt != nil {
		t.Fatalf("id/timestamp/readat: %+v", n)
	}

	for name, tc := range map[string]struct {
		uid     uuid.UUID
		kind    notification.Kind
		payload json.RawMessage
		want    error
	}{
		"nil user":      {uuid.Nil, notification.KindMessage, payload, notification.ErrInvalidUser},
		"bad kind":      {uid, notification.Kind("nope"), payload, notification.ErrInvalidKind},
		"empty payload": {uid, notification.KindMessage, json.RawMessage(``), notification.ErrInvalidPayload},
		"invalid json":  {uid, notification.KindMessage, json.RawMessage(`{`), notification.ErrInvalidPayload},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := notification.New(tc.uid, tc.kind, tc.payload, ""); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
