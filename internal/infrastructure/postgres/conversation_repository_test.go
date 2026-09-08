// internal/infrastructure/postgres/conversation_repository_test.go
package postgres

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

func within(t *testing.T, got, want time.Time, tol time.Duration) {
	t.Helper()
	if d := got.Sub(want); d < -tol || d > tol {
		t.Errorf("time %v not within %v of %v (delta %v)", got, tol, want, d)
	}
}

func TestConversationRepository_CreateGroupChatAndGetByID(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Team Standup")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("ID = %v, want %v", got.ID, c.ID)
	}
	if got.Kind != conversation.KindGroupChat {
		t.Errorf("Kind = %q, want %q", got.Kind, conversation.KindGroupChat)
	}
	if got.Title != "Team Standup" {
		t.Errorf("Title = %q, want %q", got.Title, "Team Standup")
	}
	within(t, got.CreatedAt, c.CreatedAt, time.Second)
}

func TestConversationRepository_GetByID_NotFound(t *testing.T) {
	repo := NewConversationRepository(testDB)
	_, err := repo.GetByID(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestConversationRepository_GetMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Team")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m, err := repo.GetMember(ctx, c.ID, creator.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if m.Role != conversation.RoleOwner {
		t.Errorf("Role = %q, want %q", m.Role, conversation.RoleOwner)
	}
	if m.ConversationID != c.ID || m.UserID != creator.ID {
		t.Errorf("member ids = (%v,%v), want (%v,%v)", m.ConversationID, m.UserID, c.ID, creator.ID)
	}
	if m.MutedUntil != nil {
		t.Errorf("MutedUntil = %v, want nil", m.MutedUntil)
	}
	if m.LastReadMessageID != nil {
		t.Errorf("LastReadMessageID = %v, want nil", m.LastReadMessageID)
	}
	if m.JoinedAt.IsZero() {
		t.Error("JoinedAt is zero")
	}
}

func TestConversationRepository_GetMember_NotMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)
	other := createTestUser(t)

	c, members, _ := conversation.NewGroupChat(creator.ID, "Team")
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.GetMember(ctx, c.ID, other.ID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}
}

func TestConversationRepository_CreateDirect_BothMembers(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	a := createTestUser(t)
	b := createTestUser(t)

	c, members, err := conversation.NewDirect(a.ID, b.ID)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, uid := range []uuid.UUID{a.ID, b.ID} {
		m, err := repo.GetMember(ctx, c.ID, uid)
		if err != nil {
			t.Fatalf("GetMember(%v): %v", uid, err)
		}
		if m.Role != conversation.RoleMember {
			t.Errorf("Role = %q, want %q", m.Role, conversation.RoleMember)
		}
	}
}

func TestConversationRepository_Create_RollsBackOnBadMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Rollback")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	members = append(members, &conversation.Member{
		ConversationID: c.ID,
		UserID:         uuid.Must(uuid.NewV7()), // not a real user -> FK violation
		Role:           conversation.RoleMember,
		JoinedAt:       time.Now().UTC(),
	})

	if err := repo.Create(ctx, c, members); err == nil {
		t.Fatal("Create with a non-existent member must return an error")
	}

	if _, err := repo.GetByID(ctx, c.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("expected the tx to roll back leaving no orphan conversation row (ErrNotFound), got %v", err)
	}
}

func TestConversationRepository_GetMember_NonNilOptionalFields(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	creator := createTestUser(t)

	c, members, err := conversation.NewGroupChat(creator.ID, "Optional Fields")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// last_read_message_id has an FK to messages(id), so it must point at a real
	// row; create one to mark as read.
	marker, _ := message.NewText(c.ID, creator.ID, "read marker", nil)
	if err := NewMessageRepository(testDB).Create(ctx, marker); err != nil {
		t.Fatalf("Create marker message: %v", err)
	}

	mutedUntil := time.Now().UTC().Add(24 * time.Hour)
	if _, err := testDB.ExecContext(ctx, `
		UPDATE conversation_members
		SET muted_until = $1, last_read_message_id = $2
		WHERE conversation_id = $3 AND user_id = $4`,
		mutedUntil, marker.ID, c.ID, creator.ID,
	); err != nil {
		t.Fatalf("update member optional fields: %v", err)
	}

	m, err := repo.GetMember(ctx, c.ID, creator.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if m.MutedUntil == nil {
		t.Fatal("MutedUntil is nil, want non-nil")
	}
	within(t, *m.MutedUntil, mutedUntil, time.Second)
	if m.LastReadMessageID == nil {
		t.Fatal("LastReadMessageID is nil, want non-nil")
	}
	if *m.LastReadMessageID != marker.ID {
		t.Errorf("LastReadMessageID = %v, want %v", *m.LastReadMessageID, marker.ID)
	}
}

func TestConversationRepository_ListForUser(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	me := createTestUser(t)
	other := createTestUser(t)

	c1, m1, _ := conversation.NewGroupChat(me.ID, "First")
	if err := repo.Create(ctx, c1, m1); err != nil {
		t.Fatalf("Create c1: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	c2, m2, _ := conversation.NewGroupChat(me.ID, "Second")
	if err := repo.Create(ctx, c2, m2); err != nil {
		t.Fatalf("Create c2: %v", err)
	}
	cOther, mOther, _ := conversation.NewGroupChat(other.ID, "NotMine")
	if err := repo.Create(ctx, cOther, mOther); err != nil {
		t.Fatalf("Create cOther: %v", err)
	}

	list, err := repo.ListForUser(ctx, me.ID, time.Time{}, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != c2.ID || list[1].ID != c1.ID {
		t.Errorf("order = [%v, %v], want newest-first [%v, %v]", list[0].ID, list[1].ID, c2.ID, c1.ID)
	}
}

func TestConversationRepository_GetDirect_FoundOrderIndependent(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	a := createTestUser(t)
	b := createTestUser(t)

	c, members, err := conversation.NewDirect(a.ID, b.ID)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Look it up with the arguments in the opposite order.
	got, err := repo.GetDirect(ctx, b.ID, a.ID)
	if err != nil {
		t.Fatalf("GetDirect: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("ID = %v, want %v", got.ID, c.ID)
	}
	if got.Kind != conversation.KindDirect {
		t.Errorf("Kind = %q, want %q", got.Kind, conversation.KindDirect)
	}
	within(t, got.LastMessageAt, c.CreatedAt, time.Second)
}

func TestConversationRepository_GetDirect_NotFound(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	a := createTestUser(t)
	b := createTestUser(t)

	_, err := repo.GetDirect(ctx, a.ID, b.ID)
	if !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConversationRepository_Create_DuplicateDirectIsErrDirectExists(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	a := createTestUser(t)
	b := createTestUser(t)

	c1, m1, err := conversation.NewDirect(a.ID, b.ID)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	if err := repo.Create(ctx, c1, m1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Same pair, arguments reversed -> same direct_key -> unique violation.
	c2, m2, err := conversation.NewDirect(b.ID, a.ID)
	if err != nil {
		t.Fatalf("NewDirect reversed: %v", err)
	}
	err = repo.Create(ctx, c2, m2)
	if !errors.Is(err, conversation.ErrDirectExists) {
		t.Fatalf("err = %v, want ErrDirectExists", err)
	}
}

func TestConversationRepository_ListForUser_KeysetPaginates(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	me := createTestUser(t)

	var created []*conversation.Conversation
	for i := 0; i < 3; i++ {
		c, m, err := conversation.NewGroupChat(me.ID, "chat")
		if err != nil {
			t.Fatalf("NewGroupChat: %v", err)
		}
		if err := repo.Create(ctx, c, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		created = append(created, c)
		time.Sleep(2 * time.Millisecond)
	}
	// created[2] is newest.

	page1, err := repo.ListForUser(ctx, me.ID, time.Time{}, uuid.Nil, 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != created[2].ID || page1[1].ID != created[1].ID {
		t.Fatalf("page1 ids = %v, want [%v %v]", idsOf(page1), created[2].ID, created[1].ID)
	}

	last := page1[len(page1)-1]
	page2, err := repo.ListForUser(ctx, me.ID, last.LastMessageAt, last.ID, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 || page2[0].ID != created[0].ID {
		t.Fatalf("page2 ids = %v, want [%v]", idsOf(page2), created[0].ID)
	}
}

func TestConversationRepository_ListForUser_KeysetTiebreaksOnID(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	me := createTestUser(t)

	var created []*conversation.Conversation
	for i := 0; i < 3; i++ {
		c, m, err := conversation.NewGroupChat(me.ID, "tie")
		if err != nil {
			t.Fatalf("NewGroupChat: %v", err)
		}
		if err := repo.Create(ctx, c, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		created = append(created, c)
	}

	// Force all three rows to an identical last_message_at so every keyset
	// comparison is decided by the (last_message_at, id) id-tiebreak arm.
	ts := time.Now().UTC().Truncate(time.Microsecond)
	for _, c := range created {
		if _, err := testDB.ExecContext(ctx,
			"UPDATE conversations SET last_message_at = $1 WHERE id = $2", ts, c.ID,
		); err != nil {
			t.Fatalf("force last_message_at for %v: %v", c.ID, err)
		}
	}

	// Expected order is purely id DESC now.
	ids := idsOf(created)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() > ids[j].String() })

	page1, err := repo.ListForUser(ctx, me.ID, time.Time{}, uuid.Nil, 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if got := idsOf(page1); len(got) != 2 || got[0] != ids[0] || got[1] != ids[1] {
		t.Fatalf("page1 ids = %v, want [%v %v]", got, ids[0], ids[1])
	}
	within(t, page1[0].LastMessageAt, ts, time.Millisecond)

	last := page1[len(page1)-1]
	page2, err := repo.ListForUser(ctx, me.ID, ts, last.ID, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if got := idsOf(page2); len(got) != 1 || got[0] != ids[2] {
		t.Fatalf("page2 ids = %v, want [%v]", got, ids[2])
	}

	seen := map[uuid.UUID]int{}
	for _, c := range page1 {
		seen[c.ID]++
	}
	for _, c := range page2 {
		seen[c.ID]++
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("id %v appeared %d times across both pages, want exactly 1 (pages %v / %v)",
				id, seen[id], idsOf(page1), idsOf(page2))
		}
	}
}

func idsOf(cs []*conversation.Conversation) []uuid.UUID {
	out := make([]uuid.UUID, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

// seedChannel creates a public channel of the given kind owned by a fresh user
// and returns the conversation id and the owner id.
func seedChannel(t *testing.T, kind conversation.Kind) (channelID, ownerID uuid.UUID) {
	t.Helper()
	owner := createTestUser(t)
	c, members, err := conversation.NewChannel(owner.ID, kind, "chan "+randomUsername(), true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if err := NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	return c.ID, owner.ID
}

func TestConversationRepository_CreateChannel_PublicRoundTrips(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelBroadcast)

	got, err := repo.GetByID(ctx, chID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Kind != conversation.KindChannelBroadcast || !got.Public {
		t.Fatalf("got %+v, want a public channel_broadcast", got)
	}
}

func TestConversationRepository_AddMember_AndIdempotent(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup)
	joiner := createTestUser(t)

	m := &conversation.Member{ConversationID: chID, UserID: joiner.ID, Role: conversation.RoleMember, JoinedAt: time.Now().UTC()}
	if err := repo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	got, err := repo.GetMember(ctx, chID, joiner.ID)
	if err != nil || got.Role != conversation.RoleMember {
		t.Fatalf("GetMember after AddMember = %v, %v", got, err)
	}
	// second AddMember for the same pair must not error
	if err := repo.AddMember(ctx, m); err != nil {
		t.Fatalf("idempotent AddMember: %v", err)
	}
}

func TestConversationRepository_RemoveMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, ownerID := seedChannel(t, conversation.KindChannelGroup)

	if err := repo.RemoveMember(ctx, chID, ownerID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if _, err := repo.GetMember(ctx, chID, ownerID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("GetMember after remove = %v, want ErrNotMember", err)
	}
	// removing again → ErrNotMember
	if err := repo.RemoveMember(ctx, chID, ownerID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("second RemoveMember = %v, want ErrNotMember", err)
	}
}

func TestConversationRepository_ListPublicChannels(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	owner := createTestUser(t)

	// a private channel and a group chat must NOT appear
	priv, pm, _ := conversation.NewChannel(owner.ID, conversation.KindChannelGroup, "private "+randomUsername(), false)
	if err := repo.Create(ctx, priv, pm); err != nil {
		t.Fatalf("create private: %v", err)
	}
	gc, gm, _ := conversation.NewGroupChat(owner.ID, "group "+randomUsername())
	if err := repo.Create(ctx, gc, gm); err != nil {
		t.Fatalf("create group: %v", err)
	}

	tag := randomUsername()
	var want []uuid.UUID
	for i := 0; i < 3; i++ {
		c, m, _ := conversation.NewChannel(owner.ID, conversation.KindChannelBroadcast, "News "+tag, true)
		if err := repo.Create(ctx, c, m); err != nil {
			t.Fatalf("create public #%d: %v", i, err)
		}
		want = append(want, c.ID)
		time.Sleep(2 * time.Millisecond)
	}
	// want[2] is newest.

	got, err := repo.ListPublicChannels(ctx, "news "+tag, time.Time{}, uuid.Nil, 10)
	if err != nil {
		t.Fatalf("ListPublicChannels: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (private + group_chat excluded, ILIKE matched)", len(got))
	}
	if got[0].ID != want[2] || got[2].ID != want[0] {
		t.Fatalf("order = %v, want newest-first %v", idsOf(got), []uuid.UUID{want[2], want[1], want[0]})
	}
	for _, c := range got {
		if !c.Public || c.Kind != conversation.KindChannelBroadcast {
			t.Fatalf("returned a non-public / non-channel row: %+v", c)
		}
	}

	// keyset: page of 2, then the rest
	page1, err := repo.ListPublicChannels(ctx, "news "+tag, time.Time{}, uuid.Nil, 2)
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1 = %v, %v", idsOf(page1), err)
	}
	last := page1[len(page1)-1]
	page2, err := repo.ListPublicChannels(ctx, "news "+tag, last.LastMessageAt, last.ID, 2)
	if err != nil || len(page2) != 1 || page2[0].ID != want[0] {
		t.Fatalf("page2 = %v, %v", idsOf(page2), err)
	}
}

func TestConversationRepository_ListPublicChannels_EmptyQueryMatchesAll(t *testing.T) {
	repo := NewConversationRepository(testDB)
	chID, _ := seedChannel(t, conversation.KindChannelGroup)

	got, err := repo.ListPublicChannels(context.Background(), "", time.Time{}, uuid.Nil, 100)
	if err != nil {
		t.Fatalf("ListPublicChannels: %v", err)
	}
	var found bool
	for _, c := range got {
		if c.ID == chID {
			found = true
		}
	}
	if !found {
		t.Fatalf("seeded public channel %s not returned by an empty-query list", chID)
	}
}
