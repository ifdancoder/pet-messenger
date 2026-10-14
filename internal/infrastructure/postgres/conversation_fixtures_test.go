// internal/infrastructure/postgres/conversation_fixtures_test.go
//
// Shared test fixtures for seeding an owning conversation, for tests in
// this package that need one but aren't testing the conversation
// repository itself (that lives in
// internal/conversation/adapter/out/persistence now — see the design spec
// docs/superpowers/specs/2026-10-12-conversation-clean-hexagonal-cqrs-design.md).
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	convdomain "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/conversation/adapter/out/persistence"
)

// seedGroupChat creates a group chat with a fresh creator, returns
// (convID, creatorID).
func seedGroupChat(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()
	creator := createTestUser(t)
	c, members, err := convdomain.NewGroupChat(creator.ID, "grp "+randomUsername())
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := persistence.NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed group chat: %v", err)
	}
	return c.ID, creator.ID
}

// addMember inserts a plain member into convID, optionally retiering it.
func addMember(t *testing.T, convID uuid.UUID, tier convdomain.AdminTier, joinedAt time.Time) uuid.UUID {
	t.Helper()
	u := createTestUser(t)
	repo := persistence.NewConversationRepository(testDB)
	m := &convdomain.Member{ConversationID: convID, UserID: u.ID, Role: convdomain.RoleMember, AdminTier: convdomain.TierNone, JoinedAt: joinedAt}
	if err := repo.AddMember(context.Background(), m); err != nil {
		t.Fatalf("addMember: %v", err)
	}
	if tier != convdomain.TierNone {
		if err := repo.SetMemberTier(context.Background(), convID, u.ID, tier); err != nil {
			t.Fatalf("addMember SetMemberTier: %v", err)
		}
	}
	return u.ID
}
