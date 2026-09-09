// internal/infrastructure/postgres/conversation_repository_test.go
package postgres

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

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
	if m.Role != conversation.RoleMember {
		t.Errorf("Role = %q, want %q", m.Role, conversation.RoleMember)
	}
	if !m.IsCreator || m.AdminTier != conversation.TierCreatorAdmin {
		t.Errorf("founder flags = (IsCreator %v, AdminTier %q), want (true, %q)", m.IsCreator, m.AdminTier, conversation.TierCreatorAdmin)
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
	ctx := context.Background()
	owner := createTestUser(t)

	// Seed one public channel with a fresh unique title tag so this test does
	// not depend on how many other public channels the shared table holds.
	tag := "empty-q " + randomUsername()
	c, m, err := conversation.NewChannel(owner.ID, conversation.KindChannelGroup, tag, true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if err := repo.Create(ctx, c, m); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	contains := func(cs []*conversation.Conversation, id uuid.UUID) bool {
		for _, cv := range cs {
			if cv.ID == id {
				return true
			}
		}
		return false
	}

	// Empty query: the seeded row (freshest, unique tag) must be present.
	empty, err := repo.ListPublicChannels(ctx, "", time.Time{}, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("ListPublicChannels(empty): %v", err)
	}
	if !contains(empty, c.ID) {
		t.Fatalf("seeded public channel %s not returned by an empty-query list", c.ID)
	}

	// Same row must also be reachable via its unique tag.
	byTag, err := repo.ListPublicChannels(ctx, tag, time.Time{}, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("ListPublicChannels(tag): %v", err)
	}
	if !contains(byTag, c.ID) {
		t.Fatalf("seeded public channel %s not returned by a tag query %q", c.ID, tag)
	}
}

func TestConversationRepository_Create_FounderRow(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	owner := createTestUser(t)
	c, members, err := conversation.NewGroupChat(owner.ID, "founders "+randomUsername())
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := repo.Create(ctx, c, members); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetMember(ctx, c.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if !got.IsCreator || got.AdminTier != conversation.TierCreatorAdmin {
		t.Fatalf("founder row = %+v, want IsCreator + TierCreatorAdmin", got)
	}
}

func TestConversationRepository_GetMember_TierNoneWhenNull(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup)
	joiner := createTestUser(t)
	if err := repo.AddMember(ctx, &conversation.Member{
		ConversationID: chID, UserID: joiner.ID, Role: conversation.RoleMember, JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	got, err := repo.GetMember(ctx, chID, joiner.ID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if got.IsCreator || got.AdminTier != conversation.TierNone {
		t.Fatalf("joined member = %+v, want no creator flag and TierNone", got)
	}
}

func TestConversationRepository_SetMemberTier(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup)
	joiner := createTestUser(t)
	if err := repo.AddMember(ctx, &conversation.Member{
		ConversationID: chID, UserID: joiner.ID, Role: conversation.RoleMember, JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := repo.SetMemberTier(ctx, chID, joiner.ID, conversation.TierAdmin); err != nil {
		t.Fatalf("SetMemberTier admin: %v", err)
	}
	got, _ := repo.GetMember(ctx, chID, joiner.ID)
	if got.AdminTier != conversation.TierAdmin {
		t.Fatalf("after set = %q, want admin", got.AdminTier)
	}

	if err := repo.SetMemberTier(ctx, chID, joiner.ID, conversation.TierNone); err != nil {
		t.Fatalf("SetMemberTier none: %v", err)
	}
	got, _ = repo.GetMember(ctx, chID, joiner.ID)
	if got.AdminTier != conversation.TierNone {
		t.Fatalf("after clear = %q, want TierNone", got.AdminTier)
	}

	other := uuid.Must(uuid.NewV7())
	if err := repo.SetMemberTier(ctx, chID, other, conversation.TierAdmin); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("SetMemberTier for a non-member = %v, want ErrNotMember", err)
	}
}

func TestConversationRepository_OneCreatorPerConversation(t *testing.T) {
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup)
	intruder := createTestUser(t)
	// A second is_creator row for the same conversation must violate the
	// partial unique index.
	_, err := testDB.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role, joined_at, is_creator, admin_tier)
		 VALUES ($1, $2, 'member', now(), true, 'creator_admin')`,
		chID, intruder.ID,
	)
	if err == nil {
		t.Fatal("expected a unique-violation on a second is_creator row")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("err = %v, want 23505", err)
	}
}

func TestConversationRepository_CreatorAdminOnlyCheck(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup)
	joiner := createTestUser(t)
	if err := repo.AddMember(ctx, &conversation.Member{
		ConversationID: chID, UserID: joiner.ID, Role: conversation.RoleMember, JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	// A non-creator row cannot hold creator_admin (CHECK constraint 23514).
	_, err := testDB.ExecContext(ctx,
		`UPDATE conversation_members SET admin_tier = 'creator_admin' WHERE conversation_id = $1 AND user_id = $2`,
		chID, joiner.ID,
	)
	if err == nil {
		t.Fatal("expected a CHECK violation setting creator_admin on a non-creator row")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("err = %v, want 23514 (check_violation)", err)
	}
}

// containsConv reports whether any conversation in list has the given id.
func containsConv(list []*conversation.Conversation, id uuid.UUID) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

// seedGroupChat creates a group chat with a fresh creator, returns (convID, creatorID).
func seedGroupChat(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()
	creator := createTestUser(t)
	c, members, err := conversation.NewGroupChat(creator.ID, "grp "+randomUsername())
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed group chat: %v", err)
	}
	return c.ID, creator.ID
}

// addMember inserts a plain member (helper for the succession/roster tests).
func addMember(t *testing.T, convID uuid.UUID, tier conversation.AdminTier, joinedAt time.Time) uuid.UUID {
	t.Helper()
	u := createTestUser(t)
	m := &conversation.Member{ConversationID: convID, UserID: u.ID, Role: conversation.RoleMember, AdminTier: conversation.TierNone, JoinedAt: joinedAt}
	if err := NewConversationRepository(testDB).AddMember(context.Background(), m); err != nil {
		t.Fatalf("addMember: %v", err)
	}
	if tier != conversation.TierNone {
		if err := NewConversationRepository(testDB).SetMemberTier(context.Background(), convID, u.ID, tier); err != nil {
			t.Fatalf("addMember SetMemberTier: %v", err)
		}
	}
	return u.ID
}

func TestConversationRepository_SetArchived(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, _ := seedGroupChat(t)

	if err := repo.SetArchived(ctx, convID, true); err != nil {
		t.Fatalf("SetArchived(true): %v", err)
	}
	got, _ := repo.GetByID(ctx, convID)
	if got.ArchivedAt == nil {
		t.Fatal("ArchivedAt should be set after archiving")
	}
	if err := repo.SetArchived(ctx, convID, false); err != nil {
		t.Fatalf("SetArchived(false): %v", err)
	}
	got, _ = repo.GetByID(ctx, convID)
	if got.ArchivedAt != nil {
		t.Fatal("ArchivedAt should be nil after unarchiving")
	}
	if err := repo.SetArchived(ctx, uuid.Must(uuid.NewV7()), true); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("SetArchived on a missing conversation = %v, want ErrNotFound", err)
	}
}

func TestConversationRepository_ListForUser_ExcludesArchived(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)

	list, _ := repo.ListForUser(ctx, creatorID, time.Time{}, uuid.Nil, 50)
	if !containsConv(list, convID) {
		t.Fatal("active conversation should be in the list")
	}
	if err := repo.SetArchived(ctx, convID, true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	list, _ = repo.ListForUser(ctx, creatorID, time.Time{}, uuid.Nil, 50)
	if containsConv(list, convID) {
		t.Fatal("archived conversation must be excluded from ListForUser")
	}
}

func TestConversationRepository_ListPublicChannels_ExcludesArchived(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	chID, _ := seedChannel(t, conversation.KindChannelGroup) // seedChannel makes a public channel
	if err := repo.SetArchived(ctx, chID, true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}
	list, _ := repo.ListPublicChannels(ctx, "", time.Time{}, uuid.Nil, 100)
	for _, c := range list {
		if c.ID == chID {
			t.Fatal("archived public channel must not be discoverable")
		}
	}
}

func TestConversationRepository_RemoveMemberWithSuccession(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	base := time.Now().UTC().Add(-time.Hour)
	oldPlain := addMember(t, convID, conversation.TierNone, base)                // joined first, no tier
	admin := addMember(t, convID, conversation.TierAdmin, base.Add(time.Minute)) // joined later, higher rank

	// creator leaves -> the admin (higher rank) is promoted, not the older plain member.
	promoted, err := repo.RemoveMemberWithSuccession(ctx, convID, creatorID)
	if err != nil {
		t.Fatalf("RemoveMemberWithSuccession: %v", err)
	}
	if promoted == nil || promoted.UserID != admin {
		t.Fatalf("promoted = %+v, want the admin %s", promoted, admin)
	}
	if !promoted.IsCreator || promoted.AdminTier != conversation.TierCreatorAdmin {
		t.Fatalf("promoted member not upgraded: %+v", promoted)
	}
	gotAdmin, _ := repo.GetMember(ctx, convID, admin)
	if !gotAdmin.IsCreator {
		t.Fatal("promotion not persisted")
	}
	if _, err := repo.GetMember(ctx, convID, creatorID); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatal("creator should be gone")
	}
	_ = oldPlain

	// a non-creator leaving promotes nobody.
	promoted, err = repo.RemoveMemberWithSuccession(ctx, convID, oldPlain)
	if err != nil {
		t.Fatalf("RemoveMemberWithSuccession (non-creator): %v", err)
	}
	if promoted != nil {
		t.Fatalf("promoted = %+v, want nil for a non-creator leaver", promoted)
	}

	// leaving twice -> ErrNotMember.
	if _, err := repo.RemoveMemberWithSuccession(ctx, convID, oldPlain); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("second remove = %v, want ErrNotMember", err)
	}
}

func TestConversationRepository_RemoveMemberWithSuccession_LastMember(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	promoted, err := repo.RemoveMemberWithSuccession(ctx, convID, creatorID)
	if err != nil {
		t.Fatalf("RemoveMemberWithSuccession: %v", err)
	}
	if promoted != nil {
		t.Fatalf("promoted = %+v, want nil when no members remain", promoted)
	}
}

func TestConversationRepository_ClaimCreator(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	base := time.Now().UTC().Add(-time.Hour)
	heir := addMember(t, convID, conversation.TierAdmin, base)
	plain := addMember(t, convID, conversation.TierNone, base.Add(time.Minute))

	// vacate the seat.
	if _, err := repo.RemoveMemberWithSuccession(ctx, convID, creatorID); err != nil {
		t.Fatalf("vacate: %v", err)
	}
	// the auto-promote already filled it — undo for the test: demote + clear is_creator directly.
	if _, err := testDB.ExecContext(ctx, `UPDATE conversation_members SET is_creator = false, admin_tier = 'admin' WHERE conversation_id = $1 AND is_creator`, convID); err != nil {
		t.Fatalf("reset seat: %v", err)
	}

	// plain member cannot claim over the admin heir.
	if _, err := repo.ClaimCreator(ctx, convID, plain); !errors.Is(err, conversation.ErrNotCreatorHeir) {
		t.Fatalf("plain claim = %v, want ErrNotCreatorHeir", err)
	}
	// the heir claims.
	got, err := repo.ClaimCreator(ctx, convID, heir)
	if err != nil {
		t.Fatalf("ClaimCreator: %v", err)
	}
	if !got.IsCreator || got.AdminTier != conversation.TierCreatorAdmin {
		t.Fatalf("claimed member = %+v", got)
	}
	// now the seat is taken — even a random non-member sees the taken seat first.
	if _, err := repo.ClaimCreator(ctx, convID, plain); !errors.Is(err, conversation.ErrCreatorSeatTaken) {
		t.Fatalf("claim taken seat = %v, want ErrCreatorSeatTaken", err)
	}
	if _, err := repo.ClaimCreator(ctx, convID, uuid.Must(uuid.NewV7())); !errors.Is(err, conversation.ErrCreatorSeatTaken) {
		t.Fatalf("non-member claim on a taken seat = %v, want ErrCreatorSeatTaken", err)
	}

	// ErrNotMember is the answer only for a seat-less conversation with no
	// members left at all (the heir SELECT returns no rows).
	emptyConv, emptyCreator := seedGroupChat(t)
	if _, err := repo.RemoveMemberWithSuccession(ctx, emptyConv, emptyCreator); err != nil {
		t.Fatalf("vacate empty: %v", err)
	}
	if _, err := repo.ClaimCreator(ctx, emptyConv, uuid.Must(uuid.NewV7())); !errors.Is(err, conversation.ErrNotMember) {
		t.Fatalf("claim on a member-less seat-less conversation = %v, want ErrNotMember", err)
	}
}

// TestConversationRepository_ClaimCreator_ConcurrentConverges fires two claims at
// the same seat-less conversation and asserts they converge: exactly one wins,
// the other gets a domain error, and the DB ends with exactly one is_creator row.
func TestConversationRepository_ClaimCreator_ConcurrentConverges(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	base := time.Now().UTC().Add(-time.Hour)
	a := addMember(t, convID, conversation.TierAdmin, base)
	b := addMember(t, convID, conversation.TierAdmin, base.Add(time.Minute))

	// vacate + clear the auto-promoted seat so the conversation is seat-less
	// with two equally-tiered members.
	if _, err := repo.RemoveMemberWithSuccession(ctx, convID, creatorID); err != nil {
		t.Fatalf("vacate: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`UPDATE conversation_members SET is_creator = false, admin_tier = 'admin' WHERE conversation_id = $1 AND is_creator`,
		convID,
	); err != nil {
		t.Fatalf("reset seat: %v", err)
	}

	type res struct {
		m   *conversation.Member
		err error
	}
	ch := make(chan res, 2)
	var wg sync.WaitGroup
	for _, uid := range []uuid.UUID{a, b} {
		wg.Add(1)
		go func(uid uuid.UUID) {
			defer wg.Done()
			m, err := repo.ClaimCreator(ctx, convID, uid)
			ch <- res{m, err}
		}(uid)
	}
	wg.Wait()
	close(ch)

	var winners, domainErrs int
	for r := range ch {
		switch {
		case r.err == nil && r.m != nil && r.m.IsCreator:
			winners++
		case errors.Is(r.err, conversation.ErrCreatorSeatTaken) || errors.Is(r.err, conversation.ErrNotCreatorHeir):
			domainErrs++
		default:
			t.Fatalf("unexpected ClaimCreator result: m=%+v err=%v", r.m, r.err)
		}
	}
	if winners != 1 || domainErrs != 1 {
		t.Fatalf("convergence: %d winners, %d domain errors, want 1 / 1", winners, domainErrs)
	}

	var count int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM conversation_members WHERE conversation_id = $1 AND is_creator`, convID,
	).Scan(&count); err != nil {
		t.Fatalf("count is_creator rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("is_creator rows = %d, want exactly 1", count)
	}
}

func TestConversationRepository_ListMembers(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	// The creator (seeded just now) must sort first by joined_at ASC, so the
	// extra members join strictly after it.
	base := time.Now().UTC()
	m1 := addMember(t, convID, conversation.TierNone, base.Add(1*time.Minute))
	m2 := addMember(t, convID, conversation.TierAdmin, base.Add(2*time.Minute))

	all, err := repo.ListMembers(ctx, convID, false, time.Time{}, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len = %d, want 3", len(all))
	}
	// ordered by joined_at ASC — the creator (seeded first) is index 0.
	if all[0].UserID != creatorID || all[1].UserID != m1 || all[2].UserID != m2 {
		t.Fatalf("order = %v", []uuid.UUID{all[0].UserID, all[1].UserID, all[2].UserID})
	}
	for _, mp := range all {
		if mp.Username == "" {
			t.Fatalf("member %s has no Username joined", mp.UserID)
		}
	}

	adminsOnly, err := repo.ListMembers(ctx, convID, true, time.Time{}, uuid.Nil, 50)
	if err != nil {
		t.Fatalf("ListMembers(adminsOnly): %v", err)
	}
	// creator (is_creator) + m2 (admin_tier) — not m1.
	if len(adminsOnly) != 2 {
		t.Fatalf("adminsOnly len = %d, want 2", len(adminsOnly))
	}

	// keyset: page of 1, then the rest.
	page1, err := repo.ListMembers(ctx, convID, false, time.Time{}, uuid.Nil, 1)
	if err != nil || len(page1) != 1 || page1[0].UserID != creatorID {
		t.Fatalf("page1 = %v, %v", page1, err)
	}
	last := page1[0]
	page2, err := repo.ListMembers(ctx, convID, false, last.JoinedAt, last.UserID, 10)
	if err != nil || len(page2) != 2 || page2[0].UserID != m1 {
		t.Fatalf("page2 = %v, %v", page2, err)
	}
}

func TestConversationRepository_ListMemberIDs(t *testing.T) {
	repo := NewConversationRepository(testDB)
	ctx := context.Background()
	convID, creatorID := seedGroupChat(t)
	m1 := addMember(t, convID, conversation.TierNone, time.Now().UTC())
	m2 := addMember(t, convID, conversation.TierAdmin, time.Now().UTC())

	ids, err := repo.ListMemberIDs(ctx, convID)
	if err != nil {
		t.Fatalf("ListMemberIDs: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 3 || !got[creatorID] || !got[m1] || !got[m2] {
		t.Fatalf("ids = %v, want {creator, m1, m2}", ids)
	}

	empty, err := repo.ListMemberIDs(ctx, uuid.Must(uuid.NewV7()))
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing conversation -> %v, %v (want [], nil)", empty, err)
	}
}
