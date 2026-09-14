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

// TestForwardedIDsStripInvalidSDPTokenCharacters guards against a publisher's
// browser/OS handing back a MediaStream.id or MediaStreamTrack.id containing
// characters an SDP token doesn't allow (observed in production: a
// Windows-style COM GUID wrapped in curly braces). Forwarding that raw string
// into an outgoing msid line risks a receiving browser's own SDP parser
// normalizing it differently than the roster's stream_id, desyncing the two
// values isScreenShare compares — braces must never survive into either ID.
func TestForwardedIDsStripInvalidSDPTokenCharacters(t *testing.T) {
	userID := uuid.MustParse("0196d2d5-0a5f-7000-8000-000000000001")
	braced := "{72d6f94c-8093-413a-9d49-4e216a94500c}"
	clean := "72d6f94c-8093-413a-9d49-4e216a94500c"
	if got, want := forwardedStreamID(userID, braced), "pm-0196d2d5-0a5f-7000-8000-000000000001-"+clean; got != want {
		t.Fatalf("forwardedStreamID(braced) = %q, want %q", got, want)
	}
	if got, want := forwardedTrackID(userID, "screen", braced), "pm-0196d2d5-0a5f-7000-8000-000000000001-screen-"+clean; got != want {
		t.Fatalf("forwardedTrackID(braced) = %q, want %q", got, want)
	}
}
