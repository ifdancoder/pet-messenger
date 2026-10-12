package http

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var errInvalidCursor = errors.New("http: invalid pagination cursor")

// encodeNotificationCursor packs a (created_at, id) keyset position into an
// opaque base64 token for the notification-list API.
func encodeNotificationCursor(t time.Time, id uuid.UUID) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeNotificationCursor is the inverse. An empty string is the "first page"
// sentinel and returns the zero time and uuid.Nil with no error; anything that
// does not parse returns errInvalidCursor.
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
