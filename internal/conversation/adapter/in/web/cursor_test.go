package web

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConversationCursor_RoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 8, 12, 34, 56, 123456789, time.UTC)
	id := uuid.Must(uuid.NewV7())

	enc := encodeConversationCursor(ts, id)
	if enc == "" {
		t.Fatal("encode produced empty string")
	}

	gotTS, gotID, err := decodeConversationCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotTS.Equal(ts) {
		t.Errorf("ts = %v, want %v", gotTS, ts)
	}
	if gotID != id {
		t.Errorf("id = %v, want %v", gotID, id)
	}
}

func TestConversationCursor_EmptyIsZeroNoError(t *testing.T) {
	ts, id, err := decodeConversationCursor("")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !ts.IsZero() || id != uuid.Nil {
		t.Errorf("got (%v, %v), want zero/nil", ts, id)
	}
}

func TestConversationCursor_Malformed(t *testing.T) {
	for _, bad := range []string{"!!!not base64!!!", "Zm9v", "bm90LWEtdGltZXwxMjM="} {
		if _, _, err := decodeConversationCursor(bad); err == nil {
			t.Errorf("decode(%q) = nil error, want error", bad)
		}
	}
}
