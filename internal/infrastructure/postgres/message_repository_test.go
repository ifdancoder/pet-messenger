// internal/infrastructure/postgres/message_repository_test.go
package postgres

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/message"
)

// seedConversation creates a group chat owned by a fresh user and returns the
// conversation id and the owner id.
func seedConversation(t *testing.T) (convID, ownerID uuid.UUID) {
	t.Helper()
	owner := createTestUser(t)
	c, members, err := conversation.NewGroupChat(owner.ID, "seed")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return c.ID, owner.ID
}

func TestMessageRepository_CreateTextAndGetByID(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, err := message.NewText(convID, sender, "  hello  ", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.Seq != 1 {
		t.Errorf("first message Seq = %d, want 1", m.Seq)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != m.ID || got.ConversationID != convID {
		t.Errorf("ids = (%v,%v), want (%v,%v)", got.ID, got.ConversationID, m.ID, convID)
	}
	if got.Seq != m.Seq {
		t.Errorf("got.Seq = %d, want %d", got.Seq, m.Seq)
	}
	if got.SenderID == nil || *got.SenderID != sender {
		t.Errorf("SenderID = %v, want %v", got.SenderID, sender)
	}
	if got.Type != message.TypeText {
		t.Errorf("Type = %q, want %q", got.Type, message.TypeText)
	}
	if got.Content != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}
	if got.ReplyToID != nil || got.EditedAt != nil || got.DeletedAt != nil {
		t.Errorf("expected nil reply/edited/deleted, got %v/%v/%v", got.ReplyToID, got.EditedAt, got.DeletedAt)
	}
}

func TestMessageRepository_CreateSystem_NilSender(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, _ := seedConversation(t)

	m, err := message.NewSystem(convID, "alice joined")
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SenderID != nil {
		t.Errorf("SenderID = %v, want nil for a system message", got.SenderID)
	}
	if got.Type != message.TypeSystem {
		t.Errorf("Type = %q, want %q", got.Type, message.TypeSystem)
	}
}

func TestMessageRepository_CreateWithReplyTo(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	first, _ := message.NewText(convID, sender, "first", nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.Seq != 1 {
		t.Errorf("first.Seq = %d, want 1", first.Seq)
	}
	reply, _ := message.NewText(convID, sender, "reply", &first.ID)
	if err := repo.Create(ctx, reply); err != nil {
		t.Fatalf("Create reply: %v", err)
	}
	if reply.Seq != 2 {
		t.Errorf("reply.Seq = %d, want 2", reply.Seq)
	}

	got, err := repo.GetByID(ctx, reply.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ReplyToID == nil || *got.ReplyToID != first.ID {
		t.Errorf("ReplyToID = %v, want %v", got.ReplyToID, first.ID)
	}
}

func TestMessageRepository_GetByID_NotFound(t *testing.T) {
	repo := NewMessageRepository(testDB)
	_, err := repo.GetByID(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMessageRepository_Update_PersistsEditAndDelete(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := message.NewText(convID, sender, "original", nil)
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	editAt := time.Now().UTC()
	if err := m.Edit("edited body", editAt); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("Update after edit: %v", err)
	}
	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Content != "edited body" {
		t.Errorf("Content = %q, want %q", got.Content, "edited body")
	}
	if got.EditedAt == nil {
		t.Fatal("EditedAt is nil after Update")
	}
	within(t, *got.EditedAt, editAt, time.Second)

	delAt := time.Now().UTC()
	m.MarkDeleted(delAt)
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("Update after delete: %v", err)
	}
	got, err = repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("DeletedAt is nil after Update")
	}
	if got.Content != "" {
		t.Errorf("Content = %q, want cleared after delete", got.Content)
	}
}

func TestMessageRepository_Update_NotFound(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, sender := seedConversation(t)
	m, _ := message.NewText(convID, sender, "never stored", nil)
	if err := repo.Update(context.Background(), m); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func seqOf(ms []*message.Message) []int64 {
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.Seq
	}
	return out
}

func TestMessageRepository_ListByConversation_Keyset(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	var seqs []int64
	for i := 0; i < 5; i++ {
		m, err := message.NewText(convID, sender, "m", nil)
		if err != nil {
			t.Fatalf("NewText %d: %v", i, err)
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		seqs = append(seqs, m.Seq)
	}
	if seqs[0] != 1 || seqs[4] != 5 {
		t.Fatalf("seqs = %v, want 1..5", seqs)
	}

	page, err := repo.ListByConversation(ctx, convID, 0, 3)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page) != 3 || page[0].Seq != 1 || page[2].Seq != 3 {
		t.Fatalf("page1 seqs = %v, want [1 2 3]", seqOf(page))
	}

	page, err = repo.ListByConversation(ctx, convID, seqs[2], 10)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page) != 2 || page[0].Seq != 4 || page[1].Seq != 5 {
		t.Fatalf("page2 seqs = %v, want [4 5]", seqOf(page))
	}

	page, err = repo.ListByConversation(ctx, convID, seqs[4], 10)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("page3 seqs = %v, want empty", seqOf(page))
	}

	// A negative afterSeq is clamped to 0: same result as "from the first message".
	neg, err := repo.ListByConversation(ctx, convID, -1, 10)
	if err != nil {
		t.Fatalf("afterSeq=-1: %v", err)
	}
	zero, err := repo.ListByConversation(ctx, convID, 0, 10)
	if err != nil {
		t.Fatalf("afterSeq=0: %v", err)
	}
	negSeqs, zeroSeqs := seqOf(neg), seqOf(zero)
	if len(negSeqs) != len(zeroSeqs) {
		t.Fatalf("afterSeq=-1 seqs = %v, want same as afterSeq=0 seqs %v", negSeqs, zeroSeqs)
	}
	for i := range negSeqs {
		if negSeqs[i] != zeroSeqs[i] {
			t.Fatalf("afterSeq=-1 seqs = %v, want same as afterSeq=0 seqs %v", negSeqs, zeroSeqs)
		}
	}

	otherConv, _ := seedConversation(t)
	page, err = repo.ListByConversation(ctx, otherConv, 0, 10)
	if err != nil {
		t.Fatalf("empty conv: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("empty conv seqs = %v, want empty", seqOf(page))
	}
}

func TestMessageRepository_ListByConversation_IncludesSoftDeleted(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	keep, _ := message.NewText(convID, sender, "keep", nil)
	if err := repo.Create(ctx, keep); err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	gone, _ := message.NewText(convID, sender, "gone", nil)
	if err := repo.Create(ctx, gone); err != nil {
		t.Fatalf("Create gone: %v", err)
	}
	goneSeq := gone.Seq
	gone.MarkDeleted(time.Now().UTC())
	if err := repo.Update(ctx, gone); err != nil {
		t.Fatalf("Update gone: %v", err)
	}

	page, err := repo.ListByConversation(ctx, convID, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len = %d, want 2 (soft-deleted rows are returned)", len(page))
	}
	if page[1].ID != gone.ID || !page[1].IsDeleted() || page[1].Content != "" {
		t.Errorf("deleted row = %+v, want id %v, IsDeleted, empty content", page[1], gone.ID)
	}
	if page[1].Seq != goneSeq {
		t.Errorf("soft-deleted row Seq = %d, want %d (seq survives soft-delete)", page[1].Seq, goneSeq)
	}
}

func TestMessageRepository_ListByConversation_InvalidLimit(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, _ := seedConversation(t)
	for _, lim := range []int{0, -1} {
		if _, err := repo.ListByConversation(context.Background(), convID, 0, lim); !errors.Is(err, message.ErrInvalidLimit) {
			t.Fatalf("limit %d: expected ErrInvalidLimit, got %v", lim, err)
		}
	}
}

func TestMessageRepository_Update_DoesNotResurrectSoftDeleted(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := message.NewText(convID, sender, "original", nil)
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A concurrent actor soft-deletes the row.
	deleted := *m
	deleted.MarkDeleted(time.Now().UTC())
	if err := repo.Update(ctx, &deleted); err != nil {
		t.Fatalf("Update delete: %v", err)
	}

	// A stale edit built from the pre-delete snapshot must not bring it back.
	if err := m.Edit("edited after delete", time.Now().UTC()); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := repo.Update(ctx, m); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("stale edit against a soft-deleted row: expected ErrNotFound, got %v", err)
	}

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.IsDeleted() || got.Content != "" {
		t.Errorf("row = %+v, want still soft-deleted with empty content", got)
	}
}

func TestMessageRepository_Create_AssignsMonotonicSeq(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()

	convA, senderA := seedConversation(t)
	for want := int64(1); want <= 3; want++ {
		m, err := message.NewText(convA, senderA, "a", nil)
		if err != nil {
			t.Fatalf("NewText: %v", err)
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if m.Seq != want {
			t.Errorf("convA message Seq = %d, want %d", m.Seq, want)
		}
	}

	convB, senderB := seedConversation(t)
	mb, err := message.NewText(convB, senderB, "b", nil)
	if err != nil {
		t.Fatalf("NewText b: %v", err)
	}
	if err := repo.Create(ctx, mb); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if mb.Seq != 1 {
		t.Errorf("first message of convB Seq = %d, want 1 (per-conversation restart)", mb.Seq)
	}
}

func TestMessageRepository_Create_SeqRollsBackOnInsertFailure(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	first, _ := message.NewText(convID, sender, "first", nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.Seq != 1 {
		t.Fatalf("first.Seq = %d, want 1", first.Seq)
	}

	// This message allocates a seq, then the INSERT fails: reply_to_id points at
	// a row that does not exist (FK violation on reply_to_id -> messages(id)).
	danglingReply := uuid.Must(uuid.NewV7())
	bad, _ := message.NewText(convID, sender, "boom", &danglingReply)
	if err := repo.Create(ctx, bad); err == nil {
		t.Fatal("Create with a dangling reply_to_id: expected a non-nil error")
	}

	third, _ := message.NewText(convID, sender, "third", nil)
	if err := repo.Create(ctx, third); err != nil {
		t.Fatalf("Create third: %v", err)
	}
	if third.Seq != 2 {
		t.Fatalf("third.Seq = %d, want 2 (the failed allocation rolled back)", third.Seq)
	}

	var lastSeq int64
	if err := testDB.QueryRowContext(ctx,
		`SELECT last_seq FROM conversations WHERE id = $1`, convID).Scan(&lastSeq); err != nil {
		t.Fatalf("read conversations.last_seq: %v", err)
	}
	if lastSeq != 2 {
		t.Fatalf("conversations.last_seq = %d, want 2", lastSeq)
	}
}

func TestMessageRepository_ListByConversation_OrdersBySeqNotCreatedAt(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	var ids []uuid.UUID
	var seqs []int64
	for i := 0; i < 3; i++ {
		m, err := message.NewText(convID, sender, "m", nil)
		if err != nil {
			t.Fatalf("NewText: %v", err)
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		ids = append(ids, m.ID)
		seqs = append(seqs, m.Seq)
	}

	// Force created_at into the REVERSE of seq order.
	base := time.Now().UTC()
	for i, id := range ids {
		ts := base.Add(time.Duration(-i) * time.Hour) // ids[0] newest, ids[2] oldest
		if _, err := testDB.ExecContext(ctx, `UPDATE messages SET created_at = $1 WHERE id = $2`, ts, id); err != nil {
			t.Fatalf("tamper created_at: %v", err)
		}
	}

	page, err := repo.ListByConversation(ctx, convID, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page) != 3 || page[0].Seq != seqs[0] || page[1].Seq != seqs[1] || page[2].Seq != seqs[2] {
		t.Fatalf("order = %v, want seq order %v despite reversed created_at", seqOf(page), seqs)
	}
}

func TestMessageRepository_Create_ConcurrentSendsAreGapFree(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	const n = 20
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		got   []int64
		errCh = make(chan error, n)
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := message.NewText(convID, sender, "c", nil)
			if err != nil {
				errCh <- err
				return
			}
			if err := repo.Create(ctx, m); err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			got = append(got, m.Seq)
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent Create: %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != n {
		t.Fatalf("got %d seqs, want %d", len(got), n)
	}
	for i := 0; i < n; i++ {
		if got[i] != int64(i+1) {
			t.Fatalf("seqs = %v, want exactly 1..%d (gap-free, no dupes)", got, n)
		}
	}

	page, err := repo.ListByConversation(ctx, convID, 0, 100)
	if err != nil {
		t.Fatalf("list after concurrent sends: %v", err)
	}
	if len(page) != n {
		t.Fatalf("persisted %d rows, want %d", len(page), n)
	}
	for i, m := range page {
		if m.Seq != int64(i+1) {
			t.Fatalf("persisted seqs = %v, want 1..%d in order", seqOf(page), n)
		}
	}

	var lastSeq int64
	if err := testDB.QueryRowContext(ctx,
		`SELECT last_seq FROM conversations WHERE id = $1`, convID).Scan(&lastSeq); err != nil {
		t.Fatalf("read conversations.last_seq: %v", err)
	}
	if lastSeq != int64(n) {
		t.Fatalf("conversations.last_seq = %d, want %d", lastSeq, n)
	}
}
