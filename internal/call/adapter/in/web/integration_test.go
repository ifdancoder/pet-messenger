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
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	callpersistence "pet-messenger/internal/call/adapter/out/persistence"
	callcommand "pet-messenger/internal/call/application/command"
	out "pet-messenger/internal/call/application/port/out"
	callquery "pet-messenger/internal/call/application/query"
	domain "pet-messenger/internal/call/domain"
	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	domainmsg "pet-messenger/internal/message/domain"
	domainuser "pet-messenger/internal/user/domain"
)

// setupCallIntegrationDB mirrors e2ee's own setupE2EEIntegrationDB.
// Deliberately not a TestMain: this file shares package web with
// controller_test.go's fake-based tests, which must stay Docker-free.
func setupCallIntegrationDB(t *testing.T) *sql.DB {
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

// seedCallIntegrationFixtures creates two real users and a real group
// conversation containing both, via the already-migrated conversation
// context's own persistence adapter.
func seedCallIntegrationFixtures(t *testing.T, db *sql.DB) (userA, userB, convID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	a, err := domainuser.New("it_"+uuid.NewString()[:8], "A")
	if err != nil {
		t.Fatalf("domainuser.New(a): %v", err)
	}
	a.PasswordHash = "hashed"
	b, err := domainuser.New("it_"+uuid.NewString()[:8], "B")
	if err != nil {
		t.Fatalf("domainuser.New(b): %v", err)
	}
	b.PasswordHash = "hashed"
	for _, u := range []*domainuser.User{a, b} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
			u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
		); err != nil {
			t.Fatalf("insert test user: %v", err)
		}
	}

	conv, members, err := domainconv.NewGroupChat(a.ID, "call integration group")
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	convRepo := convpersistence.NewConversationRepository(db)
	if err := convRepo.Create(ctx, conv, members); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := convRepo.AddMember(ctx, &domainconv.Member{ConversationID: conv.ID, UserID: b.ID, Role: domainconv.RoleMember, AdminTier: domainconv.TierNone, JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("add second member: %v", err)
	}

	return a.ID, b.ID, conv.ID
}

// stubMediaNodeRegistry, stubRoomControl, stubJoinTicketIssuer,
// stubICEConfigProvider, stubSystemEventPoster, and stubCallNotifier
// satisfy this context's infrastructure-heavy ports with no-op/minimal
// behavior. Their real adapters (Redis, gRPC, JWT) have no logic this
// slice's retrofit touched, so standing up the real things here would not
// add coverage -- only the real CallRepository and the real
// ConversationRepository matter for this test.
type stubMediaNodeRegistry struct{ node out.Node }

func (s stubMediaNodeRegistry) PickLeastLoaded(context.Context) (out.Node, error) { return s.node, nil }
func (s stubMediaNodeRegistry) Get(context.Context, string) (out.Node, error)     { return s.node, nil }

type stubRoomControl struct{}

func (stubRoomControl) CreateRoom(context.Context, out.Node, uuid.UUID) error { return nil }
func (stubRoomControl) CloseRoom(context.Context, out.Node, uuid.UUID) error  { return nil }
func (stubRoomControl) MuteParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stubRoomControl) UnmuteParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stubRoomControl) RemoveParticipant(context.Context, out.Node, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stubRoomControl) ListParticipants(context.Context, out.Node, uuid.UUID) ([]out.ParticipantInfo, error) {
	return nil, nil
}

type stubJoinTicketIssuer struct{}

func (stubJoinTicketIssuer) Issue(_, _ uuid.UUID, r domain.Role, _ string) (string, time.Time, error) {
	return "tkt-" + string(r), time.Now().Add(time.Minute), nil
}

type stubICEConfigProvider struct{}

func (stubICEConfigProvider) ServersFor(uuid.UUID) []out.ICEServer { return nil }

type stubSystemEventPoster struct{}

func (stubSystemEventPoster) PostSystemEvent(context.Context, *domainconv.Conversation, *uuid.UUID, domainmsg.SystemEvent, uuid.UUID, string) error {
	return nil
}

type stubCallNotifier struct{}

func (stubCallNotifier) CallStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

// newCallIntegrationHandler wires the real CallHandler: the real
// Postgres-backed call/participant persistence, the real
// Postgres-backed conversation membership check, and stub
// infrastructure-heavy ports (see above).
func newCallIntegrationHandler(db *sql.DB) *CallHandler {
	callRepo := callpersistence.NewCallRepository(db)
	convRepo := convpersistence.NewConversationRepository(db)
	nodes := stubMediaNodeRegistry{node: out.Node{ID: "node-1", WSAddr: "media:7000"}}
	rooms := stubRoomControl{}
	tickets := stubJoinTicketIssuer{}
	ice := stubICEConfigProvider{}
	events := stubSystemEventPoster{}
	notif := stubCallNotifier{}

	start := callcommand.NewStartCallHandler(callRepo, callRepo, convRepo, nodes, rooms, callRepo, tickets, ice, events, notif)
	join := callcommand.NewJoinCallHandler(callRepo, callRepo, convRepo, nodes, callRepo, tickets, ice)
	end := callcommand.NewEndCallHandler(callRepo, callRepo, convRepo, nodes, rooms, events)
	mute := callcommand.NewMuteParticipantHandler(callRepo, convRepo, nodes, rooms)
	unmute := callcommand.NewUnmuteParticipantHandler(callRepo, convRepo, nodes, rooms)
	remove := callcommand.NewRemoveParticipantHandler(callRepo, convRepo, nodes, rooms, callRepo)
	live := callquery.NewGetLiveForConversationHandler(convRepo, callRepo, callRepo, nodes, rooms, callRepo, events)

	return NewCallHandler(start, join, end, mute, unmute, remove, live)
}

// TestCallHandler_Integration_RealAdapters proves the wired
// /conversations/:id/calls, /calls/:id/join, /calls/:id/end, and
// /conversations/:id/calls/live paths work end to end against real
// Postgres: the real call/participant persistence adapter, the real
// conversation persistence adapter, the real 7 command/query handlers,
// and the real CallHandler. Requires Docker; other tests in this package
// that don't call setupCallIntegrationDB never touch it.
func TestCallHandler_Integration_RealAdapters(t *testing.T) {
	db := setupCallIntegrationDB(t)
	userA, userB, convID := seedCallIntegrationFixtures(t, db)
	h := newCallIntegrationHandler(db)

	// 1. A starts a call.
	rA := callRouter(h, &userA)
	startW := doCallJSON(t, rA, http.MethodPost, "/conversations/"+convID.String()+"/calls", "")
	if startW.Code != http.StatusCreated {
		t.Fatalf("start status = %d, body %s", startW.Code, startW.Body.String())
	}
	var startBody struct {
		Call struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"call"`
	}
	if err := json.Unmarshal(startW.Body.Bytes(), &startBody); err != nil {
		t.Fatalf("unmarshal start: %v", err)
	}
	callID := startBody.Call.ID
	if startBody.Call.Status != "active" {
		t.Fatalf("call status = %q, want active", startBody.Call.Status)
	}

	// 2. B joins as a viewer.
	rB := callRouter(h, &userB)
	joinW := doCallJSON(t, rB, http.MethodPost, "/calls/"+callID+"/join", `{"role":"viewer"}`)
	if joinW.Code != http.StatusOK {
		t.Fatalf("join status = %d, body %s", joinW.Code, joinW.Body.String())
	}

	// 3. A reads the live call back.
	liveW := doCallJSON(t, rA, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if liveW.Code != http.StatusOK {
		t.Fatalf("live status = %d, body %s", liveW.Code, liveW.Body.String())
	}
	var liveBody struct {
		Call struct {
			ID string `json:"id"`
		} `json:"call"`
	}
	if err := json.Unmarshal(liveW.Body.Bytes(), &liveBody); err != nil {
		t.Fatalf("unmarshal live: %v", err)
	}
	if liveBody.Call.ID != callID {
		t.Fatalf("live call id = %q, want %q", liveBody.Call.ID, callID)
	}

	// 4. A ends the call.
	endW := doCallJSON(t, rA, http.MethodPost, "/calls/"+callID+"/end", "")
	if endW.Code != http.StatusNoContent {
		t.Fatalf("end status = %d, body %s", endW.Code, endW.Body.String())
	}

	// 5. A second end is idempotent (204, not an error -- mutate's
	// ErrAlreadyEnded special case).
	endW2 := doCallJSON(t, rA, http.MethodPost, "/calls/"+callID+"/end", "")
	if endW2.Code != http.StatusNoContent {
		t.Fatalf("second end status = %d, want 204 (idempotent)", endW2.Code)
	}

	// 6. Live now 404s -- the call is no longer active.
	liveAfterW := doCallJSON(t, rA, http.MethodGet, "/conversations/"+convID.String()+"/calls/live", "")
	if liveAfterW.Code != http.StatusNotFound {
		t.Fatalf("live-after-end status = %d, want 404", liveAfterW.Code)
	}
}
