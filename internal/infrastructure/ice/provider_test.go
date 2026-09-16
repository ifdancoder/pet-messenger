package ice

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"slices"
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
	if len(got) != 4 {
		t.Fatalf("want stun + 3 turn entries (udp, tcp, tls), got %d: %+v", len(got), got)
	}
	wantExpiry := strconv.FormatInt(fixed.Add(time.Hour).Unix(), 10)
	mac := hmac.New(sha1.New, []byte("s3cr3t"))
	for _, turn := range got[1:] {
		if !strings.HasPrefix(turn.Username, wantExpiry+":") || !strings.HasSuffix(turn.Username, uid.String()) {
			t.Fatalf("username = %q", turn.Username)
		}
		mac.Reset()
		mac.Write([]byte(turn.Username))
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if turn.Credential != want {
			t.Fatalf("credential = %q, want %q", turn.Credential, want)
		}
	}
	// Each transport variant gets its own single-URL entry — never bundled
	// into one array — since a mixed array was observed making Firefox
	// silently drop the whole entry.
	wantURLs := [][]string{{"turn:t:3478"}, {"turn:t:3478?transport=tcp"}, {"turns:t:5349"}}
	for i, turn := range got[1:] {
		if !slices.Equal(turn.URLs, wantURLs[i]) {
			t.Fatalf("turn[%d] URLs = %v, want %v", i, turn.URLs, wantURLs[i])
		}
	}
}
