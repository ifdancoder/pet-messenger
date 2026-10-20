package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	domainnotif "pet-messenger/internal/notification/domain"
	domainpush "pet-messenger/internal/pushtoken/domain"
)

// fcmAudience is the bearer audience for a self-signed service-account JWT.
// It must be the production FCM host regardless of the (test-injectable)
// baseURL the request is actually sent to.
const fcmAudience = "https://fcm.googleapis.com/"

type FCM struct {
	projectID string
	baseURL   string // e.g. https://fcm.googleapis.com
	client    *http.Client
}

// NewFCM builds an FCM HTTP v1 sender. credentialsJSON is a service-account
// key; JWTAccessTokenSourceFromJSON signs a bearer locally (no token-endpoint
// round trip), which is a valid credential for FCM v1. baseURL "" -> production;
// a test passes an httptest.Server URL.
func NewFCM(ctx context.Context, projectID string, credentialsJSON []byte, baseURL string) (*FCM, error) {
	ts, err := google.JWTAccessTokenSourceFromJSON(credentialsJSON, fcmAudience)
	if err != nil {
		return nil, fmt.Errorf("parse FCM credentials: %w", err)
	}
	if baseURL == "" {
		baseURL = "https://fcm.googleapis.com"
	}
	return &FCM{
		projectID: projectID,
		baseURL:   baseURL,
		client:    oauth2.NewClient(ctx, ts),
	}, nil
}

func (f *FCM) Send(ctx context.Context, tok *domainpush.PushToken, n *domainnotif.Notification) error {
	body, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": tok.Token,
			"data":  map[string]string{"payload": string(Body(n))},
		},
	})
	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", f.baseURL, f.projectID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("fcm send: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	// FCM v1 error body: {"error":{"status":"NOT_FOUND"|"UNREGISTERED"|"INVALID_ARGUMENT", ...}}
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if resp.StatusCode == http.StatusNotFound || e.Error.Status == "UNREGISTERED" || e.Error.Status == "NOT_FOUND" {
		return ErrTokenGone
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" || d.ErrorCode == "INVALID_ARGUMENT" {
			return ErrTokenGone
		}
	}
	return fmt.Errorf("fcm send: status %d: %s", resp.StatusCode, raw)
}
