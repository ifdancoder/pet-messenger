package ice

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/config"
)

func TestServersFor_STUNOnly(t *testing.T) {
	p := NewProvider(&config.Config{STUNURLs: []string{"stun:a:1", "stun:b:2"}})
	got := p.ServersFor(uuid.Must(uuid.NewV7()))
	if len(got) != 1 || len(got[0].URLs) != 2 || got[0].Username != "" {
		t.Fatalf("got = %+v", got)
	}
}

func TestServersFor_TURNCreds(t *testing.T) {
	fixed := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	p := NewProvider(&config.Config{
		STUNURLs: []string{"stun:a:1"},
		TURN: config.TURNConfig{
			URL: "turn:t:3478", TLSURL: "turns:t:5349",
			Secret: "s3cr3t", CredentialTTL: time.Hour,
		},
	})
	p.now = func() time.Time { return fixed }

	uid := uuid.Must(uuid.NewV7())
	got := p.ServersFor(uid)
	if len(got) != 2 {
		t.Fatalf("want stun + turn, got %d", len(got))
	}
	turn := got[1]
	wantExpiry := strconv.FormatInt(fixed.Add(time.Hour).Unix(), 10)
	if !strings.HasPrefix(turn.Username, wantExpiry+":") || !strings.HasSuffix(turn.Username, uid.String()) {
		t.Fatalf("username = %q", turn.Username)
	}
	mac := hmac.New(sha1.New, []byte("s3cr3t"))
	mac.Write([]byte(turn.Username))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if turn.Credential != want {
		t.Fatalf("credential = %q, want %q", turn.Credential, want)
	}
	if len(turn.URLs) != 2 {
		t.Fatalf("turn URLs = %v", turn.URLs)
	}
}
