package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"pet-messenger/internal/pushtoken/domain"
)

func TestPlatform_Valid(t *testing.T) {
	for _, p := range []domain.Platform{domain.PlatformWeb, domain.PlatformFCM, domain.PlatformAPNS} {
		if !p.Valid() {
			t.Errorf("%q should be Valid", p)
		}
	}
	if domain.Platform("android").Valid() {
		t.Error("bogus platform should be invalid")
	}
}

func TestNew(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tok, err := domain.New(uid, domain.PlatformFCM, "  abc123  ")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tok.UserID != uid || tok.Platform != domain.PlatformFCM || tok.Token != "abc123" {
		t.Fatalf("tok = %+v", tok)
	}
	if tok.ID.Version() != 7 || tok.CreatedAt.IsZero() || !tok.LastUsedAt.Equal(tok.CreatedAt) {
		t.Fatalf("id/timestamps: %+v", tok)
	}
	for name, tc := range map[string]struct {
		uid   uuid.UUID
		plat  domain.Platform
		token string
		want  error
	}{
		"nil user":     {uuid.Nil, domain.PlatformWeb, "t", domain.ErrInvalidUser},
		"bad platform": {uid, domain.Platform("x"), "t", domain.ErrInvalidPlatform},
		"empty token":  {uid, domain.PlatformWeb, "   ", domain.ErrInvalidToken},
		"long token":   {uid, domain.PlatformWeb, strings.Repeat("x", 4097), domain.ErrInvalidToken},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.New(tc.uid, tc.plat, tc.token); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
