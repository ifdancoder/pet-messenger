// internal/e2ee/adapter/out/persistence/device_repository_test.go
package persistence

import (
	"context"
	"database/sql"
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

	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	domaine2ee "pet-messenger/internal/e2ee/domain"
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

func testDevice(userID uuid.UUID, marker byte) *domaine2ee.Device {
	publicKey := make([]byte, 32)
	publicKey[0] = marker
	return &domaine2ee.Device{
		ID: uuid.New(), UserID: userID, PublicKey: publicKey,
		Fingerprint: uuid.NewString(), Label: "test", CreatedAt: time.Now().UTC(),
	}
}

func TestDeviceRepository_RegisterFirstDeviceAutoTrusted(t *testing.T) {
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	d := testDevice(owner.ID, 1)
	if err := repo.Register(context.Background(), d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if d.Status != domaine2ee.StatusTrusted {
		t.Fatalf("Status = %q, want trusted", d.Status)
	}
}

func TestDeviceRepository_RegisterSecondDevicePending(t *testing.T) {
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	first := testDevice(owner.ID, 1)
	if err := repo.Register(context.Background(), first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	second := testDevice(owner.ID, 2)
	if err := repo.Register(context.Background(), second); err != nil {
		t.Fatalf("Register second: %v", err)
	}
	if second.Status != domaine2ee.StatusPending {
		t.Fatalf("Status = %q, want pending", second.Status)
	}
}

func TestDeviceRepository_Get_NotFound(t *testing.T) {
	repo := NewDeviceRepository(testDB)
	if _, err := repo.Get(context.Background(), uuid.Must(uuid.NewV7())); err != domaine2ee.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeviceRepository_ApproveTrustsPendingDevice(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	first := testDevice(owner.ID, 1)
	if err := repo.Register(ctx, first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	second := testDevice(owner.ID, 2)
	if err := repo.Register(ctx, second); err != nil {
		t.Fatalf("Register second: %v", err)
	}
	if err := repo.Approve(ctx, first.ID, second.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	got, err := repo.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domaine2ee.StatusTrusted {
		t.Fatalf("Status = %q, want trusted", got.Status)
	}
	if got.ApprovedBy == nil || *got.ApprovedBy != first.ID {
		t.Fatalf("ApprovedBy = %v, want %v", got.ApprovedBy, first.ID)
	}
}

func TestDeviceRepository_ApproveRejectsUntrustedApprover(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	first := testDevice(owner.ID, 1)
	if err := repo.Register(ctx, first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	second := testDevice(owner.ID, 2)
	if err := repo.Register(ctx, second); err != nil {
		t.Fatalf("Register second: %v", err)
	}
	third := testDevice(owner.ID, 3)
	if err := repo.Register(ctx, third); err != nil {
		t.Fatalf("Register third: %v", err)
	}
	if err := repo.Approve(ctx, second.ID, third.ID); err != domaine2ee.ErrNotTrusted {
		t.Fatalf("err = %v, want ErrNotTrusted (second is itself only pending)", err)
	}
}

func TestDeviceRepository_ListForUser(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	other := createTestUser(t)
	a := testDevice(owner.ID, 1)
	b := testDevice(owner.ID, 2)
	c := testDevice(other.ID, 3)
	for _, d := range []*domaine2ee.Device{a, b, c} {
		if err := repo.Register(ctx, d); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	got, err := repo.ListForUser(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestDeviceRepository_ListTrustedForConversation(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepository(testDB)
	firstUser := createTestUser(t)
	secondUser := createTestUser(t)

	first := testDevice(firstUser.ID, 1)
	if err := repo.Register(ctx, first); err != nil {
		t.Fatalf("register first device: %v", err)
	}
	pending := testDevice(firstUser.ID, 2)
	if err := repo.Register(ctx, pending); err != nil {
		t.Fatalf("register second device: %v", err)
	}
	if err := repo.Approve(ctx, first.ID, pending.ID); err != nil {
		t.Fatalf("approve second device: %v", err)
	}
	other := testDevice(secondUser.ID, 3)
	if err := repo.Register(ctx, other); err != nil {
		t.Fatalf("register other user's device: %v", err)
	}

	conversations := convpersistence.NewConversationRepository(testDB)
	chat, members, err := domainconv.NewGroupChat(firstUser.ID, "encrypted room")
	if err != nil {
		t.Fatalf("new group: %v", err)
	}
	if err := conversations.Create(ctx, chat, members); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := conversations.AddMember(ctx, &domainconv.Member{
		ConversationID: chat.ID, UserID: secondUser.ID, Role: domainconv.RoleMember, JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	got, err := repo.ListTrustedForConversation(ctx, chat.ID)
	if err != nil {
		t.Fatalf("ListTrustedForConversation: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("trusted device count = %d, want 3", len(got))
	}
	seen := map[uuid.UUID]bool{}
	for _, device := range got {
		seen[device.ID] = true
	}
	if !seen[first.ID] || !seen[pending.ID] || !seen[other.ID] {
		t.Fatalf("conversation device set = %v, missing a trusted device", seen)
	}
}

func TestDeviceRepository_KeyEnvelopes(t *testing.T) {
	ctx := context.Background()
	repo := NewDeviceRepository(testDB)
	owner := createTestUser(t)
	ownerDevice := testDevice(owner.ID, 1)
	if err := repo.Register(ctx, ownerDevice); err != nil {
		t.Fatalf("Register owner: %v", err)
	}
	recipient := createTestUser(t)
	recipientDevice := testDevice(recipient.ID, 2)
	if err := repo.Register(ctx, recipientDevice); err != nil {
		t.Fatalf("Register recipient: %v", err)
	}

	scope := "call:" + uuid.NewString()
	wantCiphertext := []byte("opaque call key")
	if err := repo.PutKeyEnvelopes(ctx, []*domaine2ee.KeyEnvelope{{
		OwnerDeviceID: ownerDevice.ID, RecipientDeviceID: recipientDevice.ID, Scope: scope,
		Ciphertext: wantCiphertext, CreatedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("PutKeyEnvelopes: %v", err)
	}
	envelope, err := repo.GetKeyEnvelope(ctx, recipientDevice.ID, scope)
	if err != nil {
		t.Fatalf("GetKeyEnvelope: %v", err)
	}
	if envelope.OwnerDeviceID != ownerDevice.ID || string(envelope.Ciphertext) != string(wantCiphertext) {
		t.Fatalf("envelope = %#v, want owner %s ciphertext %q", envelope, ownerDevice.ID, wantCiphertext)
	}
}

func TestDeviceRepository_GetKeyEnvelope_NotFound(t *testing.T) {
	repo := NewDeviceRepository(testDB)
	if _, err := repo.GetKeyEnvelope(context.Background(), uuid.Must(uuid.NewV7()), "call:nope"); err != domaine2ee.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
