// internal/domain/auth/session_test.go
package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/auth"
)

func TestNew(t *testing.T) {
	userID := uuid.New()
	s := auth.New(userID, "hash", "device")

	if s.UserID != userID {
		t.Errorf("UserID = %v, want %v", s.UserID, userID)
	}
	if s.RefreshTokenHash != "hash" {
		t.Errorf("RefreshTokenHash = %q, want %q", s.RefreshTokenHash, "hash")
	}
	if s.DeviceInfo != "device" {
		t.Errorf("DeviceInfo = %q, want %q", s.DeviceInfo, "device")
	}
	if s.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if s.ID.Version() != 7 {
		t.Errorf("ID version = %d, want 7 (UUIDv7)", s.ID.Version())
	}
	if s.CreatedAt.IsZero() || s.LastUsedAt.IsZero() {
		t.Error("expected timestamps to be set")
	}
	if s.IsRevoked() {
		t.Error("expected new session to not be revoked")
	}
}

func TestSession_IsRevoked(t *testing.T) {
	s := auth.New(uuid.New(), "hash", "device")
	if s.IsRevoked() {
		t.Fatal("new session should not be revoked")
	}
	now := time.Now()
	s.RevokedAt = &now
	if !s.IsRevoked() {
		t.Fatal("expected session to be revoked once RevokedAt is set")
	}
}
