// internal/auth/adapter/in/web/integration_test.go
package web

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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

	authbcrypt "pet-messenger/internal/auth/adapter/out/bcrypt"
	authjwt "pet-messenger/internal/auth/adapter/out/jwt"
	authpersistence "pet-messenger/internal/auth/adapter/out/persistence"
	authrefreshtoken "pet-messenger/internal/auth/adapter/out/refreshtoken"
	authcommand "pet-messenger/internal/auth/application/command"
	authout "pet-messenger/internal/auth/application/port/out"
	userpersistence "pet-messenger/internal/user/adapter/out/persistence"
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
			t.Errorf("terminate container: %v", err)
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

// newIntegrationRouter builds the real /auth router: real bcrypt hasher,
// real refresh-token generator, a real ed25519-backed JWT issuer, and real
// Postgres-backed user/session stores, wired by hand into the real command
// handlers and the real AuthHandler. No fx, no fakes.
func newIntegrationRouter(t *testing.T, db *sql.DB) *AuthHandler {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	hasher := authbcrypt.NewHasher()
	refreshGen := authrefreshtoken.NewGenerator()
	tokens := authjwt.NewIssuerWithKey(priv, 15*time.Minute)
	sessions := authpersistence.NewSessionRepository(db)
	users := userpersistence.NewUserRepository(db)
	ttl := authout.RefreshTTL(30 * 24 * time.Hour)

	register := authcommand.NewRegisterHandler(users, hasher, sessions, tokens, refreshGen)
	login := authcommand.NewLoginHandler(users, hasher, sessions, tokens, refreshGen)
	refresh := authcommand.NewRefreshHandler(sessions, users, tokens, refreshGen, ttl)
	logout := authcommand.NewLogoutHandler(sessions, refreshGen)

	return NewAuthHandler(register, login, refresh, logout)
}

// TestAuthHandler_Integration_RealAdapters drives the full register/login/
// refresh/logout lifecycle through the real AuthHandler, real bcrypt, real
// JWT issuance, and real Postgres-backed user/session stores: no fakes
// anywhere in the call path.
func TestAuthHandler_Integration_RealAdapters(t *testing.T) {
	db := setupIntegrationDB(t)
	h := newIntegrationRouter(t, db)
	r := newTestRouter(h)

	username := "it_" + uuid.NewString()[:8]
	const password = "correct-password-1"
	const displayName = "Integration Tester"

	// 1. Register.
	regW := doJSON(t, r, http.MethodPost, "/auth/register", map[string]string{
		"username":     username,
		"password":     password,
		"display_name": displayName,
	})
	if regW.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body: %s", regW.Code, http.StatusCreated, regW.Body.String())
	}
	var regResp authResponse
	if err := json.Unmarshal(regW.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	if regResp.AccessToken == "" {
		t.Error("register: AccessToken is empty")
	}
	if regResp.RefreshToken == "" {
		t.Error("register: RefreshToken is empty")
	}
	if regResp.User.Username != username {
		t.Errorf("register: User.Username = %q, want %q", regResp.User.Username, username)
	}
	if regResp.User.DisplayName != displayName {
		t.Errorf("register: User.DisplayName = %q, want %q", regResp.User.DisplayName, displayName)
	}

	// 2. Login with the same credentials.
	login1W := doJSON(t, r, http.MethodPost, "/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if login1W.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d, body: %s", login1W.Code, http.StatusOK, login1W.Body.String())
	}
	var login1Resp authResponse
	if err := json.Unmarshal(login1W.Body.Bytes(), &login1Resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if login1Resp.AccessToken == "" || login1Resp.RefreshToken == "" {
		t.Fatal("login: AccessToken or RefreshToken is empty")
	}
	if login1Resp.AccessToken == regResp.AccessToken {
		t.Error("login: AccessToken should differ from register's AccessToken")
	}
	if login1Resp.RefreshToken == regResp.RefreshToken {
		t.Error("login: RefreshToken should differ from register's RefreshToken")
	}

	// 3. Login with the wrong password.
	badLoginW := doJSON(t, r, http.MethodPost, "/auth/login", map[string]string{
		"username": username,
		"password": "totally-wrong-password",
	})
	if badLoginW.Code != http.StatusUnauthorized {
		t.Fatalf("login with wrong password status = %d, want %d, body: %s", badLoginW.Code, http.StatusUnauthorized, badLoginW.Body.String())
	}

	// 4. Refresh using the refresh token from step 2's login.
	refresh1W := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{
		"refresh_token": login1Resp.RefreshToken,
	})
	if refresh1W.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d, body: %s", refresh1W.Code, http.StatusOK, refresh1W.Body.String())
	}
	var refresh1Resp authResponse
	if err := json.Unmarshal(refresh1W.Body.Bytes(), &refresh1Resp); err != nil {
		t.Fatalf("unmarshal refresh response: %v", err)
	}
	if refresh1Resp.RefreshToken == "" {
		t.Fatal("refresh: RefreshToken is empty")
	}
	if refresh1Resp.RefreshToken == login1Resp.RefreshToken {
		t.Error("refresh: rotated RefreshToken should differ from the one presented")
	}

	// 5. Refresh again with the same, now-rotated-away refresh token from
	// step 2. refreshHandler.Handle (internal/auth/application/command/refresh.go)
	// treats presenting an already-rotated/revoked token as theft/replay:
	// it revokes every session for that user (including the one just
	// created in step 4) and returns ErrInvalidRefreshToken, which the
	// controller maps to 401.
	reuseW := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{
		"refresh_token": login1Resp.RefreshToken,
	})
	if reuseW.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token status = %d, want %d, body: %s", reuseW.Code, http.StatusUnauthorized, reuseW.Body.String())
	}

	// 6. Logout using the refresh token from step 4. Logout
	// (internal/auth/application/command/logout.go) is idempotent and does
	// not check whether the session is already revoked, so it succeeds
	// with 204 even though step 5's reuse detection already revoked this
	// session as collateral damage.
	logoutW := doJSON(t, r, http.MethodPost, "/auth/logout", map[string]string{
		"refresh_token": refresh1Resp.RefreshToken,
	})
	if logoutW.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d, body: %s", logoutW.Code, http.StatusNoContent, logoutW.Body.String())
	}

	// 7. Refresh again using the now-logged-out refresh token.
	postLogoutRefreshW := doJSON(t, r, http.MethodPost, "/auth/refresh", map[string]string{
		"refresh_token": refresh1Resp.RefreshToken,
	})
	if postLogoutRefreshW.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout refresh status = %d, want %d, body: %s", postLogoutRefreshW.Code, http.StatusUnauthorized, postLogoutRefreshW.Body.String())
	}
}
