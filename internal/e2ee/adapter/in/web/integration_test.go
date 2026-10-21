// internal/e2ee/adapter/in/web/integration_test.go
package web

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
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
	domaincall "pet-messenger/internal/call/domain"
	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	e2eepersistence "pet-messenger/internal/e2ee/adapter/out/persistence"
	e2eecommand "pet-messenger/internal/e2ee/application/command"
	e2eequery "pet-messenger/internal/e2ee/application/query"
	domainuser "pet-messenger/internal/user/domain"
)

// setupE2EEIntegrationDB mirrors message's setupIntegrationDB. Deliberately
// not a TestMain: this file shares package web with controller_test.go's
// fake-based tests, which must stay Docker-free.
func setupE2EEIntegrationDB(t *testing.T) *sql.DB {
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

// seedE2EEIntegrationFixtures creates two real users, a real group
// conversation containing both, and a real live call initiated by the
// first user, all via the already-migrated conversation/call contexts' own
// persistence adapters.
func seedE2EEIntegrationFixtures(t *testing.T, db *sql.DB) (userA, userB uuid.UUID, convID, callID uuid.UUID) {
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

	conv, members, err := domainconv.NewGroupChat(a.ID, "e2ee integration group")
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

	call, err := domaincall.New(conv.ID, a.ID, "node-1")
	if err != nil {
		t.Fatalf("domaincall.New: %v", err)
	}
	callRepo := callpersistence.NewCallRepository(db)
	if err := callRepo.Create(ctx, call); err != nil {
		t.Fatalf("create call: %v", err)
	}

	return a.ID, b.ID, conv.ID, call.ID
}

// newE2EEIntegrationHandler wires the real E2EEDeviceHandler: the real
// Postgres device/key-envelope adapter, and the real conversation/call
// persistence adapters satisfying e2ee's own narrow out ports.
func newE2EEIntegrationHandler(db *sql.DB) *E2EEDeviceHandler {
	deviceRepo := e2eepersistence.NewDeviceRepository(db)
	convRepo := convpersistence.NewConversationRepository(db)
	callRepo := callpersistence.NewCallRepository(db)

	register := e2eecommand.NewRegisterDeviceHandler(deviceRepo, deviceRepo)
	approve := e2eecommand.NewApproveDeviceHandler(deviceRepo, deviceRepo)
	putEnvelopes := e2eecommand.NewPutCallKeyEnvelopesHandler(deviceRepo, deviceRepo, deviceRepo, callRepo, convRepo)
	listDevices := e2eequery.NewListDevicesHandler(deviceRepo)
	listConvDevices := e2eequery.NewListConversationDevicesHandler(deviceRepo, convRepo)
	getEnvelope := e2eequery.NewGetCallKeyEnvelopeHandler(deviceRepo, deviceRepo, callRepo, convRepo)

	return NewE2EEDeviceHandler(register, approve, putEnvelopes, listDevices, listConvDevices, getEnvelope)
}

// TestE2EEDeviceHandler_Integration_RealAdapters proves the full device
// lifecycle -- register, approve, list, publish/read a call key envelope --
// works end to end against real Postgres: the real device repository, the
// real conversation/call persistence adapters, the real six command/query
// handlers, and the real E2EEDeviceHandler. Requires Docker; other tests in
// this package that don't call setupE2EEIntegrationDB never touch it.
func TestE2EEDeviceHandler_Integration_RealAdapters(t *testing.T) {
	db := setupE2EEIntegrationDB(t)
	userA, userB, convID, callID := seedE2EEIntegrationFixtures(t, db)
	h := newE2EEIntegrationHandler(db)

	// 1. Register A's first device -- auto-trusted (first device for a
	// fresh account).
	devA1 := uuid.Must(uuid.NewV7())
	pubA1 := make([]byte, 32)
	copy(pubA1, []byte("device-a1-public-key-32-bytes!!"))
	pubSignA1, privSignA1, _ := ed25519.GenerateKey(rand.Reader)
	rA := e2eeRouter(h, &userA)
	regW := doE2EEJSON(t, rA, http.MethodPost, "/users/me/e2ee/devices",
		`{"id":"`+devA1.String()+`","public_key":"`+base64.RawURLEncoding.EncodeToString(pubA1)+`","signing_public_key":"`+base64.RawURLEncoding.EncodeToString(pubSignA1)+`"}`)
	if regW.Code != http.StatusCreated {
		t.Fatalf("register A1 status = %d, body %s", regW.Code, regW.Body.String())
	}
	var devA1Resp e2eeDeviceResponse
	if err := json.Unmarshal(regW.Body.Bytes(), &devA1Resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if devA1Resp.Status != "trusted" {
		t.Fatalf("A1 status = %q, want trusted (first device)", devA1Resp.Status)
	}

	// 2. Register A's second device -- pending, needs approval.
	devA2 := uuid.Must(uuid.NewV7())
	pubA2 := make([]byte, 32)
	copy(pubA2, []byte("device-a2-public-key-32-bytes!!"))
	signA2, _, _ := ed25519.GenerateKey(rand.Reader)
	regW2 := doE2EEJSON(t, rA, http.MethodPost, "/users/me/e2ee/devices",
		`{"id":"`+devA2.String()+`","public_key":"`+base64.RawURLEncoding.EncodeToString(pubA2)+`","signing_public_key":"`+base64.RawURLEncoding.EncodeToString(signA2)+`"}`)
	if regW2.Code != http.StatusCreated {
		t.Fatalf("register A2 status = %d, body %s", regW2.Code, regW2.Body.String())
	}
	var devA2Resp e2eeDeviceResponse
	if err := json.Unmarshal(regW2.Body.Bytes(), &devA2Resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if devA2Resp.Status != "pending" {
		t.Fatalf("A2 status = %q, want pending (second device)", devA2Resp.Status)
	}

	// 3. Re-submit A1's exact registration again -- idempotent replay, 200
	// not 201.
	replayW := doE2EEJSON(t, rA, http.MethodPost, "/users/me/e2ee/devices",
		`{"id":"`+devA1.String()+`","public_key":"`+base64.RawURLEncoding.EncodeToString(pubA1)+`","signing_public_key":"`+base64.RawURLEncoding.EncodeToString(pubSignA1)+`"}`)
	if replayW.Code != http.StatusOK {
		t.Fatalf("replay A1 status = %d, want 200, body %s", replayW.Code, replayW.Body.String())
	}

	// 4. Approve A2 using A1's signature over A2's fingerprint. The exact
	// statement layout mirrors domaine2ee's private approvalStatement
	// helper: "pet-messenger:approve-device:v1:<approverDeviceID>:
	// <targetDeviceID>:<targetFingerprint>", where approverID is the
	// approving DEVICE's id (A1), not the account id.
	statement := []byte("pet-messenger:approve-device:v1:" + devA1.String() + ":" + devA2.String() + ":" + devA2Resp.Fingerprint)
	sig := ed25519.Sign(privSignA1, statement)
	approveW := doE2EEJSON(t, rA, http.MethodPost, "/users/me/e2ee/devices/"+devA2.String()+"/approve",
		`{"approver_id":"`+devA1.String()+`","signature":"`+base64.RawURLEncoding.EncodeToString(sig)+`"}`)
	if approveW.Code != http.StatusNoContent {
		t.Fatalf("approve A2 status = %d, want 204, body %s", approveW.Code, approveW.Body.String())
	}

	// 5. List A's own devices: both show up.
	listW := doE2EEJSON(t, rA, http.MethodGet, "/users/me/e2ee/devices", "")
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d", listW.Code)
	}
	var listBody struct {
		Devices []e2eeDeviceResponse `json:"devices"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listBody.Devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(listBody.Devices))
	}

	// 6. Register B's device -- auto-trusted (B's first device).
	devB := uuid.Must(uuid.NewV7())
	pubB := make([]byte, 32)
	copy(pubB, []byte("device-b-public-key-32-bytes!!!"))
	signB, _, _ := ed25519.GenerateKey(rand.Reader)
	rB := e2eeRouter(h, &userB)
	regBW := doE2EEJSON(t, rB, http.MethodPost, "/users/me/e2ee/devices",
		`{"id":"`+devB.String()+`","public_key":"`+base64.RawURLEncoding.EncodeToString(pubB)+`","signing_public_key":"`+base64.RawURLEncoding.EncodeToString(signB)+`"}`)
	if regBW.Code != http.StatusCreated {
		t.Fatalf("register B status = %d, body %s", regBW.Code, regBW.Body.String())
	}

	// 7. List conversation devices as A: all three trusted devices appear
	// (A1, A2 - now approved, and B's device), each with public key included
	// (unlike step 5's List which omits them).
	convDevicesW := doE2EEJSON(t, rA, http.MethodGet, "/conversations/"+convID.String()+"/e2ee/devices", "")
	if convDevicesW.Code != http.StatusOK {
		t.Fatalf("conversation devices status = %d, body %s", convDevicesW.Code, convDevicesW.Body.String())
	}
	var convDevicesBody struct {
		Devices []e2eeDeviceResponse `json:"devices"`
	}
	if err := json.Unmarshal(convDevicesW.Body.Bytes(), &convDevicesBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(convDevicesBody.Devices) != 3 {
		t.Fatalf("len(conversation devices) = %d, want 3 (A1, A2, B)", len(convDevicesBody.Devices))
	}
	for _, d := range convDevicesBody.Devices {
		if d.PublicKey == "" {
			t.Fatalf("conversation device %+v missing public_key", d)
		}
	}

	// 8. A (the initiator) publishes the call's session key, wrapped once
	// per trusted device (A1, A2, B).
	ct := func(label string) string {
		return base64.RawURLEncoding.EncodeToString([]byte("ciphertext-for-" + label))
	}
	envelopesJSON := `[{"recipient_device_id":"` + devA1.String() + `","ciphertext":"` + ct("a1") + `"},` +
		`{"recipient_device_id":"` + devA2.String() + `","ciphertext":"` + ct("a2") + `"},` +
		`{"recipient_device_id":"` + devB.String() + `","ciphertext":"` + ct("b") + `"}]`
	putW := doE2EEJSON(t, rA, http.MethodPut, "/calls/"+callID.String()+"/e2ee/key-envelopes",
		`{"owner_device_id":"`+devA1.String()+`","envelopes":`+envelopesJSON+`}`)
	if putW.Code != http.StatusNoContent {
		t.Fatalf("put key envelopes status = %d, want 204, body %s", putW.Code, putW.Body.String())
	}

	// 9. B reads its own wrapped copy of the call key.
	getW := doE2EEJSON(t, rB, http.MethodGet, "/calls/"+callID.String()+"/e2ee/key-envelope?device_id="+devB.String(), "")
	if getW.Code != http.StatusOK {
		t.Fatalf("get key envelope status = %d, body %s", getW.Code, getW.Body.String())
	}
	var envelopeBody map[string]string
	if err := json.Unmarshal(getW.Body.Bytes(), &envelopeBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelopeBody["owner_device_id"] != devA1.String() {
		t.Fatalf("owner_device_id = %q, want %q", envelopeBody["owner_device_id"], devA1.String())
	}
	if envelopeBody["ciphertext"] != ct("b") {
		t.Fatalf("ciphertext = %q, want B's own wrapped copy", envelopeBody["ciphertext"])
	}
	if envelopeBody["owner_public_key"] != base64.RawURLEncoding.EncodeToString(pubA1) {
		t.Fatalf("owner_public_key = %q, want A1's public key", envelopeBody["owner_public_key"])
	}
}
