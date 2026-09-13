package sfu

import (
	"testing"

	"github.com/google/uuid"
)

func TestForwardedStreamIDCarriesPublisher(t *testing.T) {
	userID := uuid.MustParse("0196d2d5-0a5f-7000-8000-000000000001")
	if got, want := forwardedStreamID(userID, "screen-123"), "pm-0196d2d5-0a5f-7000-8000-000000000001-screen-123"; got != want {
		t.Fatalf("forwardedStreamID = %q, want %q", got, want)
	}
}

func TestForwardedTrackIDCarriesMediaType(t *testing.T) {
	userID := uuid.MustParse("0196d2d5-0a5f-7000-8000-000000000001")
	if got, want := forwardedTrackID(userID, "screen", "track-123"), "pm-0196d2d5-0a5f-7000-8000-000000000001-screen-track-123"; got != want {
		t.Fatalf("forwardedTrackID = %q, want %q", got, want)
	}
}
