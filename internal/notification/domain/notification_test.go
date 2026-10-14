// internal/notification/domain/notification_test.go
package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/notification/domain"
)

func TestKind_Valid(t *testing.T) {
	for _, k := range []domain.Kind{domain.KindMessage, domain.KindMemberRemoved, domain.KindMemberTierChanged} {
		if !k.Valid() {
			t.Errorf("%q should be Valid", k)
		}
	}
	if domain.Kind("reaction").Valid() {
		t.Error(`Kind("reaction") should be invalid`)
	}
	if !domain.KindCallStarted.Valid() {
		t.Error("KindCallStarted should be Valid")
	}
}

func TestNew(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	payload := json.RawMessage(`{"conversation_id":"x"}`)

	n, err := domain.New(uid, domain.KindMessage, payload, "msg:abc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if n.UserID != uid || n.Kind != domain.KindMessage || n.DedupKey != "msg:abc" {
		t.Fatalf("n = %+v", n)
	}
	if n.ID.Version() != 7 || n.CreatedAt.IsZero() || n.ReadAt != nil {
		t.Fatalf("id/timestamp/readat: %+v", n)
	}

	for name, tc := range map[string]struct {
		uid     uuid.UUID
		kind    domain.Kind
		payload json.RawMessage
		want    error
	}{
		"nil user":      {uuid.Nil, domain.KindMessage, payload, domain.ErrInvalidUser},
		"bad kind":      {uid, domain.Kind("nope"), payload, domain.ErrInvalidKind},
		"empty payload": {uid, domain.KindMessage, json.RawMessage(``), domain.ErrInvalidPayload},
		"invalid json":  {uid, domain.KindMessage, json.RawMessage(`{`), domain.ErrInvalidPayload},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.New(tc.uid, tc.kind, tc.payload, ""); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
