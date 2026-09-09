package pushtoken_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/pushtoken"
)

func TestPlatform_Valid(t *testing.T) {
	for _, p := range []pushtoken.Platform{pushtoken.PlatformWeb, pushtoken.PlatformFCM, pushtoken.PlatformAPNS} {
		if !p.Valid() {
			t.Errorf("%q should be Valid", p)
		}
	}
	if pushtoken.Platform("android").Valid() {
		t.Error("bogus platform should be invalid")
	}
}

func TestNew(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tok, err := pushtoken.New(uid, pushtoken.PlatformFCM, "  abc123  ")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tok.UserID != uid || tok.Platform != pushtoken.PlatformFCM || tok.Token != "abc123" {
		t.Fatalf("tok = %+v", tok)
	}
	if tok.ID.Version() != 7 || tok.CreatedAt.IsZero() || !tok.LastUsedAt.Equal(tok.CreatedAt) {
		t.Fatalf("id/timestamps: %+v", tok)
	}
	for name, tc := range map[string]struct {
		uid   uuid.UUID
		plat  pushtoken.Platform
		token string
		want  error
	}{
		"nil user":     {uuid.Nil, pushtoken.PlatformWeb, "t", pushtoken.ErrInvalidUser},
		"bad platform": {uid, pushtoken.Platform("x"), "t", pushtoken.ErrInvalidPlatform},
		"empty token":  {uid, pushtoken.PlatformWeb, "   ", pushtoken.ErrInvalidToken},
		"long token":   {uid, pushtoken.PlatformWeb, strings.Repeat("x", 4097), pushtoken.ErrInvalidToken},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pushtoken.New(tc.uid, tc.plat, tc.token); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
