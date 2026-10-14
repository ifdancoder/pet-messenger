// internal/notification/adapter/in/messaging/integration_test.go

// Real-adapter end-to-end tests for the delivery worker: publish -> JetStream
// -> Worker.HandleMsg -> ProcessNotificationIntent -> Postgres / Redis
// presence / core-NATS WS / push. Deliberately a SUBSET of the exhaustive
// business-logic matrix, which lives in the fake-based
// application/command/process_notification_intent_test.go: this file proves
// only the things a fake cannot prove, namely that the wiring holds against
// real infrastructure, that dedup is enforced by a real Postgres unique
// index rather than a map, and that a real JetStream Term actually drops the
// message.
//
// There is deliberately NO package-level TestMain here: worker_test.go in
// this same package is fast and Docker-free, and a TestMain would force every
// test in the package through container startup. Each test below brings up
// its own containers through setupIntegrationInfra and tears them down with
// t.Cleanup.
package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	natsgo "github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"pet-messenger/internal/config"
	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	domainconv "pet-messenger/internal/conversation/domain"
	legacynotif "pet-messenger/internal/domain/notification"
	presenceinfra "pet-messenger/internal/infrastructure/presence"
	"pet-messenger/internal/infrastructure/push"
	outmessaging "pet-messenger/internal/notification/adapter/out/messaging"
	notificationpersistence "pet-messenger/internal/notification/adapter/out/persistence"
	notificationcommand "pet-messenger/internal/notification/application/command"
	domainnotif "pet-messenger/internal/notification/domain"
	pushtokenpersistence "pet-messenger/internal/pushtoken/adapter/out/persistence"
	domainpush "pet-messenger/internal/pushtoken/domain"
	domainuser "pet-messenger/internal/user/domain"
)

var (
	testDB   *sql.DB
	testRDB  *goredis.Client
	testJS   natsgo.JetStreamContext
	testConn *natsgo.Conn
)

func setupIntegrationInfra(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:latest",
		tcpostgres.WithDatabase("database"),
		tcpostgres.WithUsername("user"),
		tcpostgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { testDB.Close() })

	driver, err := migratepg.WithInstance(testDB, &migratepg.Config{})
	if err != nil {
		t.Fatalf("init migrate driver: %v", err)
	}
	mig, err := migrate.NewWithDatabaseInstance("file://../../../../../db/migrations", "pgx", driver)
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("run migrations: %v", err)
	}

	rc, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(rc) })
	redisURL, err := rc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	redisOpt, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	testRDB = goredis.NewClient(redisOpt)
	t.Cleanup(func() { _ = testRDB.Close() })

	nc, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("start nats container: %v", err)
	}
	t.Cleanup(func() { _ = nc.Terminate(ctx) })
	natsURL, err := nc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats connection string: %v", err)
	}
	// A freshly-booted nats server resets the first connections it accepts
	// even after the port is listening; retry briefly to absorb that window.
	var conn *natsgo.Conn
	for range 20 {
		if conn, err = natsgo.Connect(natsURL); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	testConn = conn
	testJS, err = conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}
}

// noProvidersConfig is a PushConfig with every platform's credentials absent,
// which is what push.NewMultiPusher needs to boot inside a test: Web and FCM
// stay unregistered (NewMultiPusher only registers them when their keys are
// non-empty) and PlatformAPNS resolves to the always-registered apnsStub.
// Every one of the three platforms therefore fails a Send in-process with
// push.ErrProviderNotConfigured, so no Send this test triggers can reach a
// network, whatever a recipient's presence or registered tokens look like.
// ProcessNotificationIntent logs that error and moves on (the persisted
// notification row is the source of truth), so this file asserts only which
// tokens a Send was attempted for, never a delivery outcome; the per-provider
// Send and gone-token-deletion matrix is covered by the fake-based command
// test.
func noProvidersConfig() config.PushConfig {
	return config.PushConfig{}
}

// pusherShim hands the real push.MultiPusher to this context's out.Pusher
// port, recording the token of every attempted Send on the way through.
//
// The translation step is not cosmetic and not this test's own convenience:
// internal/infrastructure/push still speaks the pre-retrofit
// internal/domain/notification.Notification, while out.Pusher was re-declared
// against internal/notification/domain.Notification, so *push.MultiPusher
// does NOT structurally satisfy out.Pusher and cannot be passed to
// NewProcessNotificationIntentHandler directly. Re-pointing
// internal/infrastructure/push at the new domain package is the cutover
// task's job (its re-grep of remaining internal/domain/notification
// importers), and the port's own doc comment claiming MultiPusher implements
// it only becomes true then; until that lands, every caller wiring a real
// MultiPusher into this use case needs a conversion like this one, the
// production fx wiring included. The two structs are field-identical, so the
// conversion is total and lossless.
type pusherShim struct {
	inner *push.MultiPusher
	mu    sync.Mutex
	sent  []string
}

func (p *pusherShim) Send(ctx context.Context, tok *domainpush.PushToken, n *domainnotif.Notification) error {
	p.mu.Lock()
	p.sent = append(p.sent, tok.Token)
	p.mu.Unlock()
	return p.inner.Send(ctx, tok, &legacynotif.Notification{
		ID: n.ID, UserID: n.UserID, Kind: legacynotif.Kind(n.Kind), Payload: n.Payload,
		DedupKey: n.DedupKey, CreatedAt: n.CreatedAt, ReadAt: n.ReadAt,
	})
}

func (p *pusherShim) sentTokens() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.sent...)
}

type env struct {
	w       *Worker
	js      natsgo.JetStreamContext
	sub     *natsgo.Subscription
	stream  string
	subjPfx string
	// ws is a core-NATS subscription on the subject space the real Producer
	// publishes WS payloads to, standing in for the per-pod Hub feed.
	ws       *natsgo.Subscription
	pusher   *pusherShim
	presence *presenceinfra.RedisPresence
}

func newEnv(t *testing.T) *env {
	t.Helper()
	setupIntegrationInfra(t)

	slug := strings.ToLower(strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	stream := "NOTIF_" + slug
	subjPfx := "notiftest." + slug

	if _, err := testJS.AddStream(&natsgo.StreamConfig{
		Name:      stream,
		Subjects:  []string{subjPfx + ".>"},
		Retention: natsgo.WorkQueuePolicy,
		Storage:   natsgo.MemoryStorage,
	}); err != nil {
		t.Fatalf("add stream %s: %v", stream, err)
	}
	t.Cleanup(func() { _ = testJS.DeleteStream(stream) })

	sub, err := testJS.PullSubscribe(subjPfx+".>", "w_"+slug,
		natsgo.AckWait(30*time.Second), natsgo.MaxDeliver(5))
	if err != nil {
		t.Fatalf("pull subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	wsSub, err := testConn.SubscribeSync("notif.ws.*")
	if err != nil {
		t.Fatalf("subscribe notif.ws.*: %v", err)
	}
	t.Cleanup(func() { _ = wsSub.Unsubscribe() })

	presence := presenceinfra.NewRedisPresence(testRDB)
	multi, err := push.NewMultiPusher(context.Background(), noProvidersConfig())
	if err != nil {
		t.Fatalf("new multi pusher: %v", err)
	}
	pusher := &pusherShim{inner: multi}
	handler := notificationcommand.NewProcessNotificationIntentHandler(
		notificationpersistence.NewNotificationRepository(testDB),
		convpersistence.NewConversationRepository(testDB),
		presence,
		outmessaging.NewProducer(testJS, testConn),
		pushtokenpersistence.NewPushTokenRepository(testDB),
		pusher,
	)
	w := NewWorker(handler)

	return &env{
		w: w, js: testJS, sub: sub, stream: stream, subjPfx: subjPfx,
		ws: wsSub, pusher: pusher, presence: presence,
	}
}

func (e *env) publishAndPull(t *testing.T, leaf string, intent outmessaging.NotifIntent) *natsgo.Msg {
	t.Helper()
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatalf("marshal intent: %v", err)
	}
	return e.publishRawAndPull(t, leaf, data)
}

func (e *env) publishRawAndPull(t *testing.T, leaf string, data []byte) *natsgo.Msg {
	t.Helper()
	if _, err := e.js.Publish(e.subjPfx+"."+leaf, data); err != nil {
		t.Fatalf("publish: %v", err)
	}
	msgs, err := e.sub.Fetch(1, natsgo.MaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("fetched %d messages, want 1", len(msgs))
	}
	return msgs[0]
}

// waitAckPending polls the JS consumer until its ack-pending count reaches
// want (m.Ack()/Term() are fire-and-forget, so the server state settles
// async).
func (e *env) waitAckPending(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	last := -1
	for time.Now().Before(deadline) {
		ci, err := e.sub.ConsumerInfo()
		if err != nil {
			t.Fatalf("consumer info: %v", err)
		}
		last = ci.NumAckPending
		if last == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ack pending = %d, want %d", last, want)
}

func (e *env) expectNoRedelivery(t *testing.T) {
	t.Helper()
	_, err := e.sub.Fetch(1, natsgo.MaxWait(1500*time.Millisecond))
	if !errors.Is(err, natsgo.ErrTimeout) {
		t.Fatalf("expected no redelivery (ErrTimeout), got err=%v", err)
	}
}

// wsUserIDs drains the core-NATS WS subscription and returns the user ID each
// payload was addressed to, decoded from the notif.ws.<uid> subject the real
// Producer publishes on.
func (e *env) wsUserIDs(t *testing.T) []uuid.UUID {
	t.Helper()
	if err := testConn.Flush(); err != nil {
		t.Fatalf("flush nats: %v", err)
	}
	var out []uuid.UUID
	for {
		m, err := e.ws.NextMsg(500 * time.Millisecond)
		if errors.Is(err, natsgo.ErrTimeout) {
			return out
		}
		if err != nil {
			t.Fatalf("ws next msg: %v", err)
		}
		uid, perr := uuid.Parse(strings.TrimPrefix(m.Subject, "notif.ws."))
		if perr != nil {
			t.Fatalf("ws subject %q: %v", m.Subject, perr)
		}
		out = append(out, uid)
	}
}

// --- db seed helpers -------------------------------------------------------

func mkUser(t *testing.T) uuid.UUID {
	t.Helper()
	u, err := domainuser.New("u_"+uuid.NewString()[:8], "Test User")
	if err != nil {
		t.Fatalf("domainuser.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.CreatedAt,
	); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

func mkGroupChat(t *testing.T, creator uuid.UUID, others ...uuid.UUID) uuid.UUID {
	t.Helper()
	c, members, err := domainconv.NewGroupChat(creator, "grp "+uuid.NewString()[:8])
	if err != nil {
		t.Fatalf("NewGroupChat: %v", err)
	}
	repo := convpersistence.NewConversationRepository(testDB)
	if err := repo.Create(context.Background(), c, members); err != nil {
		t.Fatalf("create group chat: %v", err)
	}
	for _, uid := range others {
		m := &domainconv.Member{
			ConversationID: c.ID, UserID: uid,
			Role: domainconv.RoleMember, AdminTier: domainconv.TierNone,
			JoinedAt: time.Now().UTC(),
		}
		if err := repo.AddMember(context.Background(), m); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
	}
	return c.ID
}

func mkToken(t *testing.T, uid uuid.UUID, token string) {
	t.Helper()
	pt, err := domainpush.New(uid, domainpush.PlatformFCM, token)
	if err != nil {
		t.Fatalf("pushtoken.New: %v", err)
	}
	if _, err := pushtokenpersistence.NewPushTokenRepository(testDB).Upsert(context.Background(), pt); err != nil {
		t.Fatalf("upsert token: %v", err)
	}
}

func notifCount(t *testing.T, uid uuid.UUID) int {
	t.Helper()
	var n int
	if err := testDB.QueryRow(`SELECT count(*) FROM notifications WHERE user_id = $1`, uid).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}

func tokenExists(t *testing.T, token string) bool {
	t.Helper()
	var n int
	if err := testDB.QueryRow(`SELECT count(*) FROM push_tokens WHERE token = $1`, token).Scan(&n); err != nil {
		t.Fatalf("count push_tokens: %v", err)
	}
	return n > 0
}

func strptr(s string) *string { return &s }
func i64ptr(v int64) *int64   { return &v }

// --- cases -----------------------------------------------------------------

// Group chat [A(sender), B(online), C(offline, two registered tokens)] over
// the whole real stack: a row for B and C but not A (real Postgres), one WS
// payload for B only (real Redis presence plus the real Producer on core
// NATS), and the message Ack'd on a real JetStream consumer. C's tokens are
// listed through the real pushtoken Postgres adapter and handed to the real
// MultiPusher, which rejects them in-process with ErrProviderNotConfigured
// (see noProvidersConfig); the assertions below pin down that a push provider
// failing neither loses C's notification row nor deletes C's tokens, since
// ErrProviderNotConfigured is not ErrTokenGone.
func TestIntegration_GroupChat_FansOut(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	a, b, c := mkUser(t), mkUser(t), mkUser(t)
	convID := mkGroupChat(t, a, b, c)
	if err := e.presence.Connected(ctx, b); err != nil {
		t.Fatalf("mark B online: %v", err)
	}
	mkToken(t, c, "c-tok-1")
	mkToken(t, c, "c-tok-2")

	msgID := uuid.Must(uuid.NewV7()).String()
	intent := outmessaging.NotifIntent{
		Kind: "message", ConversationID: convID.String(),
		MessageID: strptr(msgID), MessageSeq: i64ptr(7), SenderID: strptr(a.String()),
		Preview: "hi there", DedupPrefix: "msg",
	}
	m := e.publishAndPull(t, "msg."+msgID, intent)
	e.waitAckPending(t, 1)

	e.w.HandleMsg(m)

	if got := notifCount(t, a); got != 0 {
		t.Errorf("sender rows = %d, want 0", got)
	}
	if got := notifCount(t, b); got != 1 {
		t.Errorf("B rows = %d, want 1", got)
	}
	if got := notifCount(t, c); got != 1 {
		t.Errorf("C rows = %d, want 1", got)
	}
	if ids := e.wsUserIDs(t); len(ids) != 1 || ids[0] != b {
		t.Errorf("ws payloads = %v, want [B(%s)]", ids, b)
	}
	if sent := e.pusher.sentTokens(); len(sent) != 2 {
		t.Errorf("push attempts = %v, want 2 (C's two tokens, listed from real Postgres)", sent)
	}
	for _, tok := range []string{"c-tok-1", "c-tok-2"} {
		if !tokenExists(t, tok) {
			t.Errorf("%s was deleted; only push.ErrTokenGone may delete a token", tok)
		}
	}
	e.waitAckPending(t, 0)
}

// Handling the same message twice writes one row, not two: the second
// ProcessNotificationIntent attempt collides with the real
// idx_notifications_user_dedup unique index on (user_id, dedup_key). This
// test proves only the row count against that real constraint, not the
// error-mapping or Ack path; those are covered by
// notification_repository_test.go's TestNotificationRepository_CreateAndDedup
// (real ErrDuplicate) and TestIntegration_GroupChat_FansOut in this file
// (nil error implies Ack).
func TestIntegration_Redeliver_NoDuplicateRows(t *testing.T) {
	e := newEnv(t)

	a, b := mkUser(t), mkUser(t) // B offline, no registered tokens
	convID := mkGroupChat(t, a, b)

	msgID := uuid.Must(uuid.NewV7()).String()
	intent := outmessaging.NotifIntent{
		Kind: "message", ConversationID: convID.String(),
		MessageID: strptr(msgID), MessageSeq: i64ptr(1), SenderID: strptr(a.String()),
		Preview: "x", DedupPrefix: "msg",
	}
	m := e.publishAndPull(t, "msg."+msgID, intent)

	e.w.HandleMsg(m)
	if got := notifCount(t, b); got != 1 {
		t.Fatalf("after first handle: B rows = %d, want 1", got)
	}
	e.waitAckPending(t, 0)

	e.w.HandleMsg(m) // redelivery
	if got := notifCount(t, b); got != 1 {
		t.Errorf("after redeliver: B rows = %d, want 1 (dedup)", got)
	}
	e.waitAckPending(t, 0)
}

// Undecodable JSON is Term'd, not Nak'd: only a real JetStream consumer shows
// that the message is gone for good rather than redelivered up to MaxDeliver.
func TestIntegration_BadJSON_Terminated(t *testing.T) {
	e := newEnv(t)

	m := e.publishRawAndPull(t, "msg.bad", []byte("{not json"))
	e.w.HandleMsg(m)

	e.waitAckPending(t, 0)
	e.expectNoRedelivery(t)

	si, err := e.js.StreamInfo(e.stream)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if si.State.Msgs != 0 {
		t.Errorf("stream still holds %d messages, want 0 (Term drops it)", si.State.Msgs)
	}
}
