package persistence

import (
	"context"
	"errors"
	"log"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"database/sql"

	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
	domainuser "pet-messenger/internal/user/domain"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:latest",
		tcpostgres.WithDatabase("database"),
		tcpostgres.WithUsername("user"),
		tcpostgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	driver, err := migratepg.WithInstance(testDB, &migratepg.Config{})
	if err != nil {
		log.Fatalf("init migrate driver: %v", err)
	}
	mig, err := migrate.NewWithDatabaseInstance("file://../../../../../db/migrations", "pgx", driver)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	code := m.Run()

	testDB.Close()
	if err := container.Terminate(ctx); err != nil {
		log.Fatalf("terminate container: %v", err)
	}

	os.Exit(code)
}

func randomUsername() string {
	return "u_" + uuid.NewString()[:8]
}

func createTestUser(t *testing.T) *domainuser.User {
	t.Helper()
	u, err := domainuser.New(randomUsername(), "Test User")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
	); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u
}

// within asserts got is within tol of want.
func within(t *testing.T, got, want time.Time, tol time.Duration) {
	t.Helper()
	if d := got.Sub(want); d < -tol || d > tol {
		t.Errorf("time %v not within %v of %v (delta %v)", got, tol, want, d)
	}
}

// seedConversation creates a group chat owned by a fresh user and returns the
// conversation id and the owner id.
func seedConversation(t *testing.T) (convID, ownerID uuid.UUID) {
	t.Helper()
	owner := createTestUser(t)
	c, members, err := domainconv.NewGroupChat(owner.ID, "seed")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := convpersistence.NewConversationRepository(testDB).Create(context.Background(), c, members); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return c.ID, owner.ID
}

func TestMessageRepository_CreateTextAndGetByID(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, err := domainmsg.NewText(convID, sender, "  hello  ", nil)
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
	if got.Type != domainmsg.TypeText {
		t.Errorf("Type = %q, want %q", got.Type, domainmsg.TypeText)
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

	m, err := domainmsg.NewSystem(convID, "alice joined")
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
	if got.Type != domainmsg.TypeSystem {
		t.Errorf("Type = %q, want %q", got.Type, domainmsg.TypeSystem)
	}
}

func TestMessageRepository_CreateWithReplyTo(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	first, _ := domainmsg.NewText(convID, sender, "first", nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.Seq != 1 {
		t.Errorf("first.Seq = %d, want 1", first.Seq)
	}
	reply, _ := domainmsg.NewText(convID, sender, "reply", &first.ID)
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
	if !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMessageRepository_Update_PersistsEditAndDelete(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := domainmsg.NewText(convID, sender, "original", nil)
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
	m, _ := domainmsg.NewText(convID, sender, "never stored", nil)
	if err := repo.Update(context.Background(), m); !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func seqOf(ms []*domainmsg.Message) []int64 {
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
		m, err := domainmsg.NewText(convID, sender, "m", nil)
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

	keep, _ := domainmsg.NewText(convID, sender, "keep", nil)
	if err := repo.Create(ctx, keep); err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	gone, _ := domainmsg.NewText(convID, sender, "gone", nil)
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
		if _, err := repo.ListByConversation(context.Background(), convID, 0, lim); !errors.Is(err, domainmsg.ErrInvalidLimit) {
			t.Fatalf("limit %d: expected ErrInvalidLimit, got %v", lim, err)
		}
	}
}

func TestMessageRepository_Update_DoesNotResurrectSoftDeleted(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := domainmsg.NewText(convID, sender, "original", nil)
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
	if err := repo.Update(ctx, m); !errors.Is(err, domainmsg.ErrNotFound) {
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
		m, err := domainmsg.NewText(convA, senderA, "a", nil)
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
	mb, err := domainmsg.NewText(convB, senderB, "b", nil)
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

	first, _ := domainmsg.NewText(convID, sender, "first", nil)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.Seq != 1 {
		t.Fatalf("first.Seq = %d, want 1", first.Seq)
	}

	// This message allocates a seq, then the INSERT fails: reply_to_id points at
	// a row that does not exist (FK violation on reply_to_id -> messages(id)).
	danglingReply := uuid.Must(uuid.NewV7())
	bad, _ := domainmsg.NewText(convID, sender, "boom", &danglingReply)
	if err := repo.Create(ctx, bad); err == nil {
		t.Fatal("Create with a dangling reply_to_id: expected a non-nil error")
	}

	third, _ := domainmsg.NewText(convID, sender, "third", nil)
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
		m, err := domainmsg.NewText(convID, sender, "m", nil)
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
			m, err := domainmsg.NewText(convID, sender, "c", nil)
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

func TestMessageRepository_ListBeforeSeq_NewestFirst(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	for i := 0; i < 5; i++ {
		m, err := domainmsg.NewText(convID, sender, "m", nil)
		if err != nil {
			t.Fatalf("NewText %d: %v", i, err)
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	page, err := repo.ListBeforeSeq(ctx, convID, 0, 3)
	if err != nil {
		t.Fatalf("ListBeforeSeq: %v", err)
	}
	if got := seqOf(page); len(got) != 3 || got[0] != 5 || got[1] != 4 || got[2] != 3 {
		t.Fatalf("seqs = %v, want [5 4 3] (newest first)", got)
	}
}

func TestMessageRepository_ListBeforeSeq_PagesUpward(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	for i := 0; i < 5; i++ {
		m, _ := domainmsg.NewText(convID, sender, "m", nil)
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	page, err := repo.ListBeforeSeq(ctx, convID, 3, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq: %v", err)
	}
	if got := seqOf(page); len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("seqs = %v, want [2 1] (before seq 3, newest first)", got)
	}
}

func TestMessageRepository_ListBeforeSeq_BeforeOldest(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	m, _ := domainmsg.NewText(convID, sender, "only", nil)
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	page, err := repo.ListBeforeSeq(ctx, convID, 1, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("seqs = %v, want empty (nothing before seq 1)", seqOf(page))
	}
}

func TestMessageRepository_ListBeforeSeq_EmptyConversation(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, _ := seedConversation(t)

	page, err := repo.ListBeforeSeq(context.Background(), convID, 0, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("seqs = %v, want empty", seqOf(page))
	}
}

func TestMessageRepository_ListBeforeSeq_InvalidLimit(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, _ := seedConversation(t)
	for _, lim := range []int{0, -1} {
		if _, err := repo.ListBeforeSeq(context.Background(), convID, 0, lim); !errors.Is(err, domainmsg.ErrInvalidLimit) {
			t.Fatalf("limit %d: expected ErrInvalidLimit, got %v", lim, err)
		}
	}
}

func TestMessageRepository_ListBeforeSeq_IncludesSoftDeleted(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	keep, _ := domainmsg.NewText(convID, sender, "keep", nil)
	if err := repo.Create(ctx, keep); err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	gone, _ := domainmsg.NewText(convID, sender, "gone", nil)
	if err := repo.Create(ctx, gone); err != nil {
		t.Fatalf("Create gone: %v", err)
	}
	goneSeq := gone.Seq
	gone.MarkDeleted(time.Now().UTC())
	if err := repo.Update(ctx, gone); err != nil {
		t.Fatalf("Update gone: %v", err)
	}

	page, err := repo.ListBeforeSeq(ctx, convID, 0, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len = %d, want 2 (soft-deleted rows are returned)", len(page))
	}
	if page[0].Seq != goneSeq || !page[0].IsDeleted() || page[0].Content != "" {
		t.Errorf("newest row = %+v, want soft-deleted seq %d with empty content", page[0], goneSeq)
	}
}

func TestMessageRepository_ListBeforeSeq_NegativeBeforeSeqSameAsZero(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	for i := 0; i < 3; i++ {
		m, _ := domainmsg.NewText(convID, sender, "m", nil)
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	zero, err := repo.ListBeforeSeq(ctx, convID, 0, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq(0): %v", err)
	}
	neg, err := repo.ListBeforeSeq(ctx, convID, -1, 10)
	if err != nil {
		t.Fatalf("ListBeforeSeq(-1): %v", err)
	}
	if a, b := seqOf(zero), seqOf(neg); len(a) != len(b) || a[0] != b[0] || a[len(a)-1] != b[len(b)-1] {
		t.Fatalf("negative beforeSeq (%v) must match zero (%v)", b, a)
	}
}

func TestMessageRepository_Create_AdvancesLastMessageAt(t *testing.T) {
	ctx := context.Background()
	msgRepo := NewMessageRepository(testDB)
	convRepo := convpersistence.NewConversationRepository(testDB)
	convID, sender := seedConversation(t)

	before, err := convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("GetByID before: %v", err)
	}

	m, err := domainmsg.NewText(convID, sender, "hello", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	if err := msgRepo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	after, err := convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("GetByID after: %v", err)
	}
	if !after.LastMessageAt.After(before.LastMessageAt) {
		t.Fatalf("last_message_at not advanced: before=%v after=%v", before.LastMessageAt, after.LastMessageAt)
	}
	within(t, after.LastMessageAt, m.CreatedAt, time.Second)
}

func TestMessageRepository_Create_LastMessageAtIsMonotonic(t *testing.T) {
	ctx := context.Background()
	msgRepo := NewMessageRepository(testDB)
	convRepo := convpersistence.NewConversationRepository(testDB)
	convID, sender := seedConversation(t)

	recent, err := domainmsg.NewText(convID, sender, "recent", nil)
	if err != nil {
		t.Fatalf("NewText recent: %v", err)
	}
	if err := msgRepo.Create(ctx, recent); err != nil {
		t.Fatalf("Create recent: %v", err)
	}
	afterRecent, err := convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// A message whose CreatedAt is in the past must not drag last_message_at back.
	stale, err := domainmsg.NewText(convID, sender, "stale", nil)
	if err != nil {
		t.Fatalf("NewText stale: %v", err)
	}
	stale.CreatedAt = afterRecent.LastMessageAt.Add(-time.Hour)
	if err := msgRepo.Create(ctx, stale); err != nil {
		t.Fatalf("Create stale: %v", err)
	}

	afterStale, err := convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !afterStale.LastMessageAt.Equal(afterRecent.LastMessageAt) {
		t.Fatalf("last_message_at moved backwards: was %v now %v", afterRecent.LastMessageAt, afterStale.LastMessageAt)
	}
}

func TestMessageRepository_Create_IdempotencyKeyDeduplicates(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	key := "send-" + randomUsername()
	first, err := domainmsg.NewText(convID, sender, "hello", nil)
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	first.IdempotencyKey = &key
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if first.Seq != 1 {
		t.Fatalf("first Seq = %d, want 1", first.Seq)
	}

	// A retry with the same key: Create rejects it and consumes no seq.
	retry, err := domainmsg.NewText(convID, sender, "hello", nil)
	if err != nil {
		t.Fatalf("NewText retry: %v", err)
	}
	retry.IdempotencyKey = &key
	if err := repo.Create(ctx, retry); !errors.Is(err, domainmsg.ErrDuplicateIdempotencyKey) {
		t.Fatalf("retry Create err = %v, want ErrDuplicateIdempotencyKey", err)
	}

	got, err := repo.GetByIdempotencyKey(ctx, convID, sender, key)
	if err != nil {
		t.Fatalf("GetByIdempotencyKey: %v", err)
	}
	if got.ID != first.ID || got.Seq != 1 {
		t.Fatalf("GetByIdempotencyKey returned id=%v seq=%d, want id=%v seq=1", got.ID, got.Seq, first.ID)
	}

	// The rejected retry burned no seq: a fresh keyless send gets seq 2.
	next, err := domainmsg.NewText(convID, sender, "again", nil)
	if err != nil {
		t.Fatalf("NewText next: %v", err)
	}
	if err := repo.Create(ctx, next); err != nil {
		t.Fatalf("next Create: %v", err)
	}
	if next.Seq != 2 {
		t.Fatalf("next Seq = %d, want 2 (the duplicate consumed no seq)", next.Seq)
	}
}

func TestMessageRepository_Create_NilIdempotencyKeyNeverCollides(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, sender := seedConversation(t)

	for i := 0; i < 3; i++ {
		m, err := domainmsg.NewText(convID, sender, "m", nil)
		if err != nil {
			t.Fatalf("NewText: %v", err)
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create #%d (nil key): %v", i, err)
		}
	}
}

func TestMessageRepository_GetByIdempotencyKey_NotFound(t *testing.T) {
	repo := NewMessageRepository(testDB)
	convID, sender := seedConversation(t)
	if _, err := repo.GetByIdempotencyKey(context.Background(), convID, sender, "nope"); !errors.Is(err, domainmsg.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMessageRepository_Create_SameKeyDifferentSenders(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, senderA := seedConversation(t)
	// a second member who can post
	userB := createTestUser(t)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, role, joined_at) VALUES ($1, $2, 'member', now())`,
		convID, userB.ID); err != nil {
		t.Fatalf("add member B: %v", err)
	}
	key := "shared-" + randomUsername()
	a, _ := domainmsg.NewText(convID, senderA, "from A", nil)
	a.IdempotencyKey = &key
	b, _ := domainmsg.NewText(convID, userB.ID, "from B", nil)
	b.IdempotencyKey = &key
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create B with same key but different sender must succeed: %v", err)
	}
	gotA, err := repo.GetByIdempotencyKey(ctx, convID, senderA, key)
	if err != nil || gotA.ID != a.ID {
		t.Fatalf("GetByIdempotencyKey(A) = %v, %v; want a.ID=%v", gotA, err, a.ID)
	}
	gotB, err := repo.GetByIdempotencyKey(ctx, convID, userB.ID, key)
	if err != nil || gotB.ID != b.ID {
		t.Fatalf("GetByIdempotencyKey(B) = %v, %v; want b.ID=%v", gotB, err, b.ID)
	}
}

func TestMessageRepository_Create_SystemEvent(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convID, actorID := seedConversation(t)
	target := createTestUser(t)

	m, err := domainmsg.NewSystemEvent(convID, &actorID, domainmsg.SystemMemberRemoved, target.ID, "")
	if err != nil {
		t.Fatalf("NewSystemEvent: %v", err)
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SystemEvent != domainmsg.SystemMemberRemoved || got.SystemTargetID == nil || *got.SystemTargetID != target.ID {
		t.Fatalf("round-trip = %+v", got)
	}
	if got.Seq <= 0 {
		t.Fatalf("system event got no seq: %d", got.Seq)
	}
}

func TestMessages_SystemEventCheck(t *testing.T) {
	convID, _ := seedConversation(t)
	// a non-system row with system_event set must violate messages_system_event_type.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO messages (id, conversation_id, type, content, seq, created_at, system_event)
		 VALUES ($1, $2, 'text', 'x', 999999, now(), 'member_joined')`,
		uuid.Must(uuid.NewV7()), convID,
	)
	if err == nil {
		t.Fatal("expected a CHECK violation")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("err = %v, want 23514", err)
	}
}

func TestMessageRepository_Create_SameKeyDifferentConversations(t *testing.T) {
	repo := NewMessageRepository(testDB)
	ctx := context.Background()
	convA, senderA := seedConversation(t)
	convB, senderB := seedConversation(t)

	key := "shared-" + randomUsername()
	a, _ := domainmsg.NewText(convA, senderA, "a", nil)
	a.IdempotencyKey = &key
	b, _ := domainmsg.NewText(convB, senderB, "b", nil)
	b.IdempotencyKey = &key

	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create in convA: %v", err)
	}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("Create in convB with the same key must succeed (index is per conversation): %v", err)
	}
}
