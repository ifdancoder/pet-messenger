package persistence

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	domain "pet-messenger/internal/call/domain"
	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
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

// seedGroupChat creates a real user and a real group chat they created,
// via the already-migrated conversation context's own persistence
// adapter, and returns (conversationID, creatorID).
func seedGroupChat(t *testing.T) (convID, creatorID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	u, err := domainuser.New("it_"+uuid.NewString()[:8], "Creator")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	conv, members, err := domainconv.NewGroupChat(u.ID, "call repository test group")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	if err := convpersistence.NewConversationRepository(testDB).Create(ctx, conv, members); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv.ID, u.ID
}

// addMember adds a fresh member to convID at the given admin tier and
// returns their id.
func addMember(t *testing.T, convID uuid.UUID, tier domainconv.AdminTier) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	u, err := domainuser.New("it_"+uuid.NewString()[:8], "Member")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
	); err != nil {
		t.Fatalf("insert test member: %v", err)
	}
	member := &domainconv.Member{ConversationID: convID, UserID: u.ID, Role: domainconv.RoleMember, AdminTier: tier, JoinedAt: time.Now().UTC()}
	if err := convpersistence.NewConversationRepository(testDB).AddMember(ctx, member); err != nil {
		t.Fatalf("add member: %v", err)
	}
	return u.ID
}

func newLiveCall(t *testing.T) (*domain.Call, uuid.UUID) {
	t.Helper()
	convID, creatorID := seedGroupChat(t)
	c, err := domain.New(convID, creatorID, "node-test")
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	if err := NewCallRepository(testDB).Create(context.Background(), c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c, creatorID
}

func TestCallRepository_CreateAndGet(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, _ := newLiveCall(t)

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != c.ID || got.Status != domain.StatusActive || got.MediaNodeID != "node-test" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := repo.GetByID(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing GetByID: %v", err)
	}
}

func TestCallRepository_OneLivePerConversation(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, creatorID := newLiveCall(t)

	dup, _ := domain.New(c.ConversationID, creatorID, "node-test")
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("second live call for the same conversation must fail")
	}

	live, err := repo.GetLiveForConversation(ctx, c.ConversationID)
	if err != nil || live.ID != c.ID {
		t.Fatalf("GetLiveForConversation = %v, %v", live, err)
	}

	// After the first ends, a new live call is allowed.
	if _, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkEnded: %v", err)
	}
	if _, err := repo.GetLiveForConversation(ctx, c.ConversationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetLiveForConversation after end: %v", err)
	}
	if err := repo.Create(ctx, dup); err != nil {
		t.Fatalf("Create after previous ended: %v", err)
	}
}

func TestCallRepository_MarkEndedIdempotent(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, _ := newLiveCall(t)

	first, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil || !first {
		t.Fatalf("first MarkEnded = %v, %v (want true, nil)", first, err)
	}
	second, err := repo.MarkEnded(ctx, c.ID, time.Now().UTC())
	if err != nil || second {
		t.Fatalf("second MarkEnded = %v, %v (want false, nil)", second, err)
	}
}

func TestCallRepository_Participants(t *testing.T) {
	repo := NewCallRepository(testDB)
	ctx := context.Background()
	c, creatorID := newLiveCall(t)
	other := addMember(t, c.ConversationID, domainconv.TierNone)

	p1, _ := domain.NewParticipant(c.ID, creatorID, domain.RolePublisher)
	if err := repo.AddParticipant(ctx, p1); err != nil {
		t.Fatalf("AddParticipant p1: %v", err)
	}
	dup, _ := domain.NewParticipant(c.ID, creatorID, domain.RoleViewer)
	if err := repo.AddParticipant(ctx, dup); !errors.Is(err, domain.ErrAlreadyInCall) {
		t.Fatalf("dup AddParticipant: %v, want ErrAlreadyInCall", err)
	}
	p2, _ := domain.NewParticipant(c.ID, other, domain.RoleViewer)
	if err := repo.AddParticipant(ctx, p2); err != nil {
		t.Fatalf("AddParticipant p2: %v", err)
	}

	list, err := repo.ListParticipants(ctx, c.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListParticipants = %d, %v", len(list), err)
	}

	if err := repo.MarkParticipantLeft(ctx, c.ID, creatorID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkParticipantLeft: %v", err)
	}
	// Idempotent: a second call (no active row) is fine.
	if err := repo.MarkParticipantLeft(ctx, c.ID, creatorID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkParticipantLeft again: %v", err)
	}
	// The creator may re-join now that the active row is closed.
	rejoin, _ := domain.NewParticipant(c.ID, creatorID, domain.RolePublisher)
	if err := repo.AddParticipant(ctx, rejoin); err != nil {
		t.Fatalf("re-join after leave: %v", err)
	}
}
