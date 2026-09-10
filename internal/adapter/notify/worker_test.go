package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
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

	domainconv "pet-messenger/internal/domain/conversation"
	domainnotif "pet-messenger/internal/domain/notification"
	"pet-messenger/internal/domain/pushtoken"
	domainuser "pet-messenger/internal/domain/user"
	notifinfra "pet-messenger/internal/infrastructure/notification"
	"pet-messenger/internal/infrastructure/postgres"
	presenceinfra "pet-messenger/internal/infrastructure/presence"
	"pet-messenger/internal/infrastructure/push"
)

var (
	testDB  *sql.DB
	testRDB *goredis.Client
	testJS  natsgo.JetStreamContext
)

func TestMain(m *testing.M) {
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
		log.Fatalf("start postgres container: %v", err)
	}
	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("postgres connection string: %v", err)
	}
	testDB, err = sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	driver, err := migratepg.WithInstance(testDB, &migratepg.Config{})
	if err != nil {
		log.Fatalf("init migrate driver: %v", err)
	}
	mig, err := migrate.NewWithDatabaseInstance("file://../../../db/migrations", "pgx", driver)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	rc, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		log.Fatalf("start redis container: %v", err)
	}
	redisURL, err := rc.ConnectionString(ctx)
	if err != nil {
		log.Fatalf("redis connection string: %v", err)
	}
	redisOpt, err := goredis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}
	testRDB = goredis.NewClient(redisOpt)

	nc, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		log.Fatalf("start nats container: %v", err)
	}
	natsURL, err := nc.ConnectionString(ctx)
	if err != nil {
		log.Fatalf("nats connection string: %v", err)
	}
	// A freshly-booted nats server resets the first connections it accepts even
	// after the port is listening; retry briefly to absorb that window.
	var conn *natsgo.Conn
	for range 20 {
		if conn, err = natsgo.Connect(natsURL); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	testJS, err = conn.JetStream()
	if err != nil {
		log.Fatalf("jetstream context: %v", err)
	}

	code := m.Run()

	testDB.Close()
	conn.Close()
	_ = pg.Terminate(ctx)
	_ = testcontainers.TerminateContainer(rc)
	_ = nc.Terminate(ctx)
	os.Exit(code)
}

// --- fakes -------------------------------------------------------------------

type pushCall struct {
	token   string
	notifID uuid.UUID
}

type fakePusher struct {
	mu    sync.Mutex
	calls []pushCall
	gone  map[string]bool // token -> report ErrTokenGone
}

func (f *fakePusher) Send(_ context.Context, tok *pushtoken.PushToken, n *domainnotif.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pushCall{token: tok.Token, notifID: n.ID})
	if f.gone[tok.Token] {
		return push.ErrTokenGone
	}
	return nil
}

func (f *fakePusher) sentTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.token
	}
	return out
}

type wsCall struct {
	uid     uuid.UUID
	payload []byte
}

type fakeWS struct {
	mu    sync.Mutex
	calls []wsCall
}

func (f *fakeWS) Notify(uid uuid.UUID, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, wsCall{uid: uid, payload: append([]byte(nil), payload...)})
	return nil
}

func (f *fakeWS) userIDs() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]uuid.UUID, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.uid
	}
	return out
}

// --- per-test harness ------------------------------------------------------

type env struct {
	w        *Worker
	js       natsgo.JetStreamContext
	sub      *natsgo.Subscription
	stream   string
	subjPfx  string
	pusher   *fakePusher
	ws       *fakeWS
	presence *presenceinfra.RedisPresence
}

func newEnv(t *testing.T) *env {
	t.Helper()
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

	pusher := &fakePusher{gone: map[string]bool{}}
	ws := &fakeWS{}
	presence := presenceinfra.NewRedisPresence(testRDB)

	w := NewWorker(
		postgres.NewNotificationRepository(testDB),
		postgres.NewPushTokenRepository(testDB),
		postgres.NewConversationRepository(testDB),
		presence,
		pusher,
		ws,
	)
	return &env{
		w: w, js: testJS, sub: sub, stream: stream, subjPfx: subjPfx,
		pusher: pusher, ws: ws, presence: presence,
	}
}

func (e *env) publishAndPull(t *testing.T, leaf string, intent notifinfra.NotifIntent) *natsgo.Msg {
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

// waitAckPending polls the JS consumer until its ack-pending count reaches want
// (m.Ack()/Term() are fire-and-forget, so the server state settles async).
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

// --- db seed helpers -----------------------------------------------------

func mkUser(t *testing.T) uuid.UUID {
	t.Helper()
	u, err := domainuser.New("u_"+uuid.NewString()[:8], "Test User")
	if err != nil {
		t.Fatalf("user.New: %v", err)
	}
	u.PasswordHash = "hashed"
	if err := postgres.NewUserRepository(testDB).Create(context.Background(), u); err != nil {
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
	repo := postgres.NewConversationRepository(testDB)
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
	pt, err := pushtoken.New(uid, pushtoken.PlatformFCM, token)
	if err != nil {
		t.Fatalf("pushtoken.New: %v", err)
	}
	if _, err := postgres.NewPushTokenRepository(testDB).Upsert(context.Background(), pt); err != nil {
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

// --- cases -------------------------------------------------------------------

//  1. group chat [A(sender), B(online), C(offline)] -> row for B and C (not A);
//     WS once for B; push for each of C's tokens; message Ack'd.
func TestWorker_GroupChat_FansOut(t *testing.T) {
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
	intent := notifinfra.NotifIntent{
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
	if ids := e.ws.userIDs(); len(ids) != 1 || ids[0] != b {
		t.Errorf("ws calls = %v, want [B(%s)]", ids, b)
	}
	sent := e.pusher.sentTokens()
	if len(sent) != 2 {
		t.Errorf("pusher calls = %v, want 2 (C's tokens)", sent)
	}
	e.waitAckPending(t, 0)
}

// 2. redeliver the same *natsgo.Msg -> no new rows, still Ack.
func TestWorker_Redeliver_NoDuplicateRows(t *testing.T) {
	e := newEnv(t)

	a, b := mkUser(t), mkUser(t) // B offline
	convID := mkGroupChat(t, a, b)
	mkToken(t, b, "b-redeliver-tok")

	msgID := uuid.Must(uuid.NewV7()).String()
	intent := notifinfra.NotifIntent{
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

//  3. a token the pusher reports ErrTokenGone for is deleted; a healthy token is
//     left alone.
func TestWorker_GoneToken_Deleted(t *testing.T) {
	e := newEnv(t)

	a, c := mkUser(t), mkUser(t) // C offline
	convID := mkGroupChat(t, a, c)
	mkToken(t, c, "gone-tok")
	mkToken(t, c, "live-tok")
	e.pusher.gone["gone-tok"] = true

	msgID := uuid.Must(uuid.NewV7()).String()
	intent := notifinfra.NotifIntent{
		Kind: "message", ConversationID: convID.String(),
		MessageID: strptr(msgID), MessageSeq: i64ptr(3), SenderID: strptr(a.String()),
		Preview: "y", DedupPrefix: "msg",
	}
	m := e.publishAndPull(t, "msg."+msgID, intent)

	e.w.HandleMsg(m)

	if tokenExists(t, "gone-tok") {
		t.Errorf("gone-tok should have been deleted")
	}
	if !tokenExists(t, "live-tok") {
		t.Errorf("live-tok should have been kept")
	}
	if got := notifCount(t, c); got != 1 {
		t.Errorf("C rows = %d, want 1", got)
	}
	e.waitAckPending(t, 0)
}

// 4. bad JSON -> Term (no row, not redelivered).
func TestWorker_BadJSON_Terminated(t *testing.T) {
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

//  5. member_removed single-target intent (D offline) -> one row for D, push
//     sent, Ack.
func TestWorker_MemberEvent_SingleTarget(t *testing.T) {
	e := newEnv(t)

	d := mkUser(t) // offline
	mkToken(t, d, "d-tok")

	convID := uuid.Must(uuid.NewV7()).String()
	intent := notifinfra.NotifIntent{
		Kind: "member_removed", ConversationID: convID,
		RecipientID: strptr(d.String()), DedupPrefix: "kick",
	}
	m := e.publishAndPull(t, "member."+uuid.NewString(), intent)

	e.w.HandleMsg(m)

	if got := notifCount(t, d); got != 1 {
		t.Errorf("D rows = %d, want 1", got)
	}
	if sent := e.pusher.sentTokens(); len(sent) != 1 || sent[0] != "d-tok" {
		t.Errorf("pusher calls = %v, want [d-tok]", sent)
	}
	e.waitAckPending(t, 0)
}

//  7. call_started group intent [A(initiator), B, C] -> one call_started row for
//     B and C (not A); a redeliver of the same message adds no rows.
func TestWorker_CallStarted_FansOut(t *testing.T) {
	e := newEnv(t)

	a, b, c := mkUser(t), mkUser(t), mkUser(t)
	convID := mkGroupChat(t, a, b, c)

	callID := uuid.Must(uuid.NewV7()).String()
	intent := notifinfra.NotifIntent{
		Kind: "call_started", ConversationID: convID.String(),
		SenderID: strptr(a.String()), CallID: callID, DedupPrefix: "call",
	}
	m := e.publishAndPull(t, "call."+callID, intent)

	e.w.HandleMsg(m)

	if got := notifCount(t, a); got != 0 {
		t.Errorf("initiator rows = %d, want 0", got)
	}
	for _, uid := range []uuid.UUID{b, c} {
		if got := notifCount(t, uid); got != 1 {
			t.Errorf("member %s rows = %d, want 1", uid, got)
		}
		var kind string
		if err := testDB.QueryRow(`SELECT kind FROM notifications WHERE user_id = $1`, uid).Scan(&kind); err != nil {
			t.Fatalf("select kind: %v", err)
		}
		if kind != "call_started" {
			t.Errorf("member %s kind = %q, want call_started", uid, kind)
		}
	}
	e.waitAckPending(t, 0)

	e.w.HandleMsg(m) // redelivery
	for _, uid := range []uuid.UUID{b, c} {
		if got := notifCount(t, uid); got != 1 {
			t.Errorf("member %s rows after redeliver = %d, want 1 (dedup)", uid, got)
		}
	}
	e.waitAckPending(t, 0)
}

// 6. (controller ruling) unknown Kind -> no row written, message Ack'd.
func TestWorker_UnknownKind_DroppedNoRow(t *testing.T) {
	e := newEnv(t)

	d := mkUser(t)
	mkToken(t, d, "garbage-tok")

	intent := notifinfra.NotifIntent{
		Kind: "garbage", ConversationID: uuid.Must(uuid.NewV7()).String(),
		RecipientID: strptr(d.String()), DedupPrefix: "x",
	}
	m := e.publishAndPull(t, "member."+uuid.NewString(), intent)

	e.w.HandleMsg(m)

	if got := notifCount(t, d); got != 0 {
		t.Errorf("unknown-kind wrote %d rows, want 0", got)
	}
	if sent := e.pusher.sentTokens(); len(sent) != 0 {
		t.Errorf("pusher called %v, want none", sent)
	}
	e.waitAckPending(t, 0)
	e.expectNoRedelivery(t)
}
