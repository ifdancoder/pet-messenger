// internal/message/adapter/in/web/integration_test.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	natsgo "github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	messagemessaging "pet-messenger/internal/message/adapter/out/messaging"
	messagepersistence "pet-messenger/internal/message/adapter/out/persistence"
	messagecommand "pet-messenger/internal/message/application/command"
	messagequery "pet-messenger/internal/message/application/query"
	domainmsg "pet-messenger/internal/message/domain"
	domainuser "pet-messenger/internal/user/domain"
)

// setupIntegrationDB starts a throwaway, migrated Postgres instance via
// testcontainers for this one test and registers its teardown with t.
// Deliberately NOT a TestMain: this file shares package web with
// controller_test.go's fake-based tests, which must stay Docker-free.
func setupIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
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
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Errorf("terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		t.Fatalf("init migrate driver: %v", err)
	}
	mig, err := migrate.NewWithDatabaseInstance("file://../../../../../db/migrations", "pgx", driver)
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("run migrations: %v", err)
	}

	return db
}

// setupIntegrationNATS starts a throwaway core-NATS instance for this one
// test and registers its teardown with t. Also deliberately NOT a TestMain.
func setupIntegrationNATS(t *testing.T) *natsgo.Conn {
	t.Helper()
	ctx := context.Background()

	container, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("start nats container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Errorf("terminate nats container: %v", err)
		}
	})

	url, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats connection string: %v", err)
	}

	var nc *natsgo.Conn
	var lastErr error
	for range 20 {
		nc, lastErr = natsgo.Connect(url)
		if lastErr == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("connect nats: %v", lastErr)
	}
	t.Cleanup(nc.Close)

	return nc
}

// noopNotificationProducer discards every call. The real
// internal/infrastructure/notification.Producer needs a JetStream stream
// bootstrapped by fx-internal, unexported plumbing — out of proportion for
// what this integration test is closing (the real wired message path
// through Postgres and core NATS, not the notification subsystem, which
// has its own slice later in the retrofit roadmap).
type noopNotificationProducer struct{}

func (noopNotificationProducer) MessageSent(context.Context, *domainmsg.Message, *domainconv.Conversation) error {
	return nil
}
func (noopNotificationProducer) MembershipEvent(context.Context, domainmsg.SystemEvent, uuid.UUID, uuid.UUID, string) error {
	return nil
}

// seedIntegrationConversation creates a real user and a real group chat
// with a second member, via the already-migrated conversation context's
// own persistence adapter, and returns (conversationID, ownerID, otherID).
func seedIntegrationConversation(t *testing.T, db *sql.DB) (convID, ownerID, otherID uuid.UUID) {
	t.Helper()
	owner, err := domainuser.New("it_"+uuid.NewString()[:8], "Owner")
	if err != nil {
		t.Fatalf("domainuser.New(owner): %v", err)
	}
	owner.PasswordHash = "hashed"
	other, err := domainuser.New("it_"+uuid.NewString()[:8], "Other")
	if err != nil {
		t.Fatalf("domainuser.New(other): %v", err)
	}
	other.PasswordHash = "hashed"
	for _, u := range []*domainuser.User{owner, other} {
		if _, err := db.ExecContext(context.Background(),
			`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
			u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
		); err != nil {
			t.Fatalf("insert test user: %v", err)
		}
	}

	conv, members, err := domainconv.NewGroupChat(owner.ID, "integration test group")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	convRepo := convpersistence.NewConversationRepository(db)
	if err := convRepo.Create(context.Background(), conv, members); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	otherMember := &domainconv.Member{ConversationID: conv.ID, UserID: other.ID, Role: domainconv.RoleMember, AdminTier: domainconv.TierNone, JoinedAt: time.Now().UTC()}
	if err := convRepo.AddMember(context.Background(), otherMember); err != nil {
		t.Fatalf("add second member: %v", err)
	}
	return conv.ID, owner.ID, other.ID
}

// newIntegrationHandler wires the real MessageHandler: real Postgres-backed
// message and conversation persistence, a real core-NATS publisher, and a
// no-op notification producer (see noopNotificationProducer).
func newIntegrationHandler(db *sql.DB, nc *natsgo.Conn) *MessageHandler {
	msgRepo := messagepersistence.NewMessageRepository(db)
	convRepo := convpersistence.NewConversationRepository(db)
	publisher := messagemessaging.NewPublisher(nc)
	notif := noopNotificationProducer{}

	send := messagecommand.NewSendMessageHandler(msgRepo, msgRepo, convRepo, publisher, notif)
	edit := messagecommand.NewEditMessageHandler(msgRepo, msgRepo, convRepo, publisher)
	del := messagecommand.NewDeleteMessageHandler(msgRepo, msgRepo, convRepo, publisher)
	markRead := messagecommand.NewMarkReadHandler(msgRepo, convRepo, convRepo)
	list := messagequery.NewListMessagesHandler(msgRepo, msgRepo, convRepo, convRepo)

	return NewMessageHandler(send, edit, del, list, markRead)
}

// TestMessageHandler_Integration_RealAdapters proves the wired
// /conversations/:id/... path works end to end against real Postgres and
// real core NATS: the real message and conversation persistence adapters,
// the real NATS publisher, the real 5 command/query handlers, and the real
// MessageHandler. Only the notification producer is a fake (see
// noopNotificationProducer). Requires Docker; skipped tests in this package
// that don't call setupIntegrationDB/setupIntegrationNATS never touch it.
func TestMessageHandler_Integration_RealAdapters(t *testing.T) {
	db := setupIntegrationDB(t)
	nc := setupIntegrationNATS(t)
	convID, owner, other := seedIntegrationConversation(t, db)

	h := newIntegrationHandler(db, nc)
	r := msgRouter(h, &owner)

	// Subscribe to this conversation's real NATS subject before sending, so
	// we can prove the real publisher path fires, not just that Postgres
	// persisted the row.
	received := make(chan []byte, 4)
	sub, err := nc.Subscribe("msg."+convID.String(), func(msg *natsgo.Msg) { received <- msg.Data })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// 1. Send a message as the owner.
	sendW := doMsg(t, r, http.MethodPost, "/conversations/"+convID.String()+"/messages", `{"content":"hello from owner"}`)
	if sendW.Code != http.StatusCreated {
		t.Fatalf("send status = %d, body: %s", sendW.Code, sendW.Body.String())
	}
	var sent messageResponse
	if err := json.Unmarshal(sendW.Body.Bytes(), &sent); err != nil {
		t.Fatalf("unmarshal send response: %v", err)
	}
	if sent.Content != "hello from owner" || sent.Seq != 1 {
		t.Fatalf("sent = %+v", sent)
	}

	select {
	case data := <-received:
		var env struct {
			Type    string `json:"type"`
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal published event: %v", err)
		}
		if env.Type != "message.created" || env.Message.ID != sent.ID {
			t.Fatalf("published event = %+v", env)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the real NATS publish")
	}

	// 2. Edit it.
	editW := doMsg(t, r, http.MethodPatch, "/conversations/"+convID.String()+"/messages/"+sent.ID, `{"content":"edited by owner"}`)
	if editW.Code != http.StatusOK {
		t.Fatalf("edit status = %d, body: %s", editW.Code, editW.Body.String())
	}
	var edited messageResponse
	if err := json.Unmarshal(editW.Body.Bytes(), &edited); err != nil {
		t.Fatalf("unmarshal edit response: %v", err)
	}
	if edited.Content != "edited by owner" {
		t.Fatalf("edited = %+v", edited)
	}

	// 3. The other member marks it read.
	otherRouter := msgRouter(h, &other)
	markReadW := doMsg(t, otherRouter, http.MethodPost, "/conversations/"+convID.String()+"/read", `{"message_id":"`+sent.ID+`"}`)
	if markReadW.Code != http.StatusNoContent {
		t.Fatalf("mark-read status = %d, body: %s", markReadW.Code, markReadW.Body.String())
	}

	// 4. List as the owner: the read receipt from the real conversation
	// roster should show up via the real ConversationRosterPort.
	listW := doMsg(t, r, http.MethodGet, "/conversations/"+convID.String()+"/messages", "")
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, body: %s", listW.Code, listW.Body.String())
	}
	var listBody messageListResponse
	if err := json.Unmarshal(listW.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listBody.Messages) != 1 || listBody.Messages[0].Content != "edited by owner" {
		t.Fatalf("list = %+v", listBody.Messages)
	}
	if listBody.Messages[0].ReadBy != 1 || listBody.Messages[0].RecipientCount != 1 {
		t.Fatalf("read status = %+v, want ReadBy=1 RecipientCount=1 (the other member read it)", listBody.Messages[0])
	}

	// 5. Delete it.
	delW := doMsg(t, r, http.MethodDelete, "/conversations/"+convID.String()+"/messages/"+sent.ID, "")
	if delW.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body: %s", delW.Code, delW.Body.String())
	}
	var tomb messageResponse
	if err := json.Unmarshal(delW.Body.Bytes(), &tomb); err != nil {
		t.Fatalf("unmarshal delete response: %v", err)
	}
	if tomb.Content != "" || tomb.DeletedAt == nil {
		t.Fatalf("tombstone = %+v", tomb)
	}
}
