package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"pet-messenger/internal/domain/pushtoken"
)

// fakeServiceAccountJSON builds a syntactically real service-account key with a
// throwaway RSA private key. JWTAccessTokenSourceFromJSON signs the bearer
// locally from this, so the FCM test needs no Google network.
func fakeServiceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sa, err := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  "proj",
		"private_key_id":              "test-key-id",
		"private_key":                 string(keyPEM),
		"client_email":                "fcm@proj.iam.gserviceaccount.com",
		"client_id":                   "0",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	})
	if err != nil {
		t.Fatal(err)
	}
	return sa
}

// TestFCM_Send asserts NewFCM targets /v1/projects/<id>/messages:send and maps
// the response: 2xx -> nil, UNREGISTERED/NOT_FOUND -> ErrTokenGone,
// anything else -> a plain (non-ErrTokenGone) error.
func TestFCM_Send(t *testing.T) {
	sa := fakeServiceAccountJSON(t)
	n := mkNotif(t)
	tok := &pushtoken.PushToken{Platform: pushtoken.PlatformFCM, Token: "device-token"}

	tests := []struct {
		name      string
		status    int
		body      string
		wantErrIs error
		wantOK    bool
	}{
		{"ok", http.StatusOK, `{"name":"projects/proj/messages/1"}`, nil, true},
		{"unregistered", http.StatusNotFound, `{"error":{"status":"UNREGISTERED","code":404}}`, ErrTokenGone, false},
		{"invalid argument detail", http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT","details":[{"errorCode":"INVALID_ARGUMENT"}]}}`, ErrTokenGone, false},
		{"server error", http.StatusInternalServerError, `{"error":{"status":"INTERNAL"}}`, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			f, err := NewFCM(context.Background(), "proj", sa, srv.URL)
			if err != nil {
				t.Fatalf("NewFCM: %v", err)
			}
			err = f.Send(context.Background(), tok, n)

			if gotPath != "/v1/projects/proj/messages:send" {
				t.Fatalf("request path = %q", gotPath)
			}
			if gotAuth == "" {
				t.Fatal("no Authorization header attached")
			}
			switch {
			case tc.wantOK:
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
			case tc.wantErrIs != nil:
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tc.wantErrIs)
				}
			default:
				if err == nil || errors.Is(err, ErrTokenGone) {
					t.Fatalf("want a plain error, got %v", err)
				}
			}
		})
	}
}

func TestNewFCM_BadCredentials(t *testing.T) {
	if _, err := NewFCM(context.Background(), "proj", []byte(`{"type":"not_a_service_account"}`), ""); err == nil {
		t.Fatal("want error for non service-account credentials")
	}
}
