package postgres

import (
	"context"
	"testing"

	"pet-messenger/internal/domain/pushtoken"
)

func TestPushTokenRepository(t *testing.T) {
	repo := NewPushTokenRepository(testDB)
	ctx := context.Background()
	u := createTestUser(t)

	tk, _ := pushtoken.New(u.ID, pushtoken.PlatformFCM, "tok-1")
	got, err := repo.Upsert(ctx, tk)
	if err != nil || got.Token != "tok-1" || got.Platform != pushtoken.PlatformFCM {
		t.Fatalf("Upsert = %+v, %v", got, err)
	}

	// conflict refreshes last_used_at, does not duplicate
	tk2, _ := pushtoken.New(u.ID, pushtoken.PlatformFCM, "tok-1")
	if _, err := repo.Upsert(ctx, tk2); err != nil {
		t.Fatalf("Upsert conflict: %v", err)
	}
	list, _ := repo.ListForUser(ctx, u.ID)
	if len(list) != 1 {
		t.Fatalf("ListForUser len = %d, want 1", len(list))
	}

	// DeleteByToken idempotent
	if err := repo.DeleteByToken(ctx, u.ID, "tok-1"); err != nil {
		t.Fatalf("DeleteByToken: %v", err)
	}
	if err := repo.DeleteByToken(ctx, u.ID, "tok-1"); err != nil {
		t.Fatalf("DeleteByToken (missing): %v", err)
	}
	if list, _ := repo.ListForUser(ctx, u.ID); len(list) != 0 {
		t.Fatalf("after delete len = %d", len(list))
	}

	// DeleteToken (owner-agnostic)
	tk3, _ := pushtoken.New(u.ID, pushtoken.PlatformWeb, "tok-web")
	_, _ = repo.Upsert(ctx, tk3)
	if err := repo.DeleteToken(ctx, "tok-web"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if list, _ := repo.ListForUser(ctx, u.ID); len(list) != 0 {
		t.Fatalf("after DeleteToken len = %d", len(list))
	}
}
