package config_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"pet-messenger/internal/config"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	t.Setenv("PORT", "8080")
	t.Setenv("BLUEPRINT_DB_HOST", "localhost")
	t.Setenv("BLUEPRINT_DB_PORT", "5432")
	t.Setenv("BLUEPRINT_DB_DATABASE", "blueprint")
	t.Setenv("BLUEPRINT_DB_USERNAME", "user")
	t.Setenv("BLUEPRINT_DB_PASSWORD", "pass")
	t.Setenv("BLUEPRINT_DB_SCHEMA", "public")
	t.Setenv("JWT_ED25519_SEED_BASE64", base64.StdEncoding.EncodeToString(priv.Seed()))
}

func TestLoad(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DB.DSN() != "postgres://user:pass@localhost:5432/blueprint?sslmode=disable&search_path=public" {
		t.Errorf("unexpected DSN: %s", cfg.DB.DSN())
	}
	if len(cfg.JWT.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("PrivateKey size = %d, want %d", len(cfg.JWT.PrivateKey), ed25519.PrivateKeySize)
	}
	if len(cfg.JWT.PublicKey) != ed25519.PublicKeySize {
		t.Errorf("PublicKey size = %d, want %d", len(cfg.JWT.PublicKey), ed25519.PublicKeySize)
	}
	if cfg.JWT.AccessTTL <= 0 || cfg.JWT.RefreshTTL <= 0 {
		t.Error("expected positive TTLs")
	}
}

func TestLoad_MissingSeed(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_ED25519_SEED_BASE64", "")

	if _, err := config.Load(); err == nil {
		t.Fatal("expected error when JWT_ED25519_SEED_BASE64 is unset")
	}
}

func TestLoad_InvalidSeedLength(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_ED25519_SEED_BASE64", base64.StdEncoding.EncodeToString([]byte("too-short")))

	if _, err := config.Load(); err == nil {
		t.Fatal("expected error for a seed of the wrong length")
	}
}
