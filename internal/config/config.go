package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

type DBConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	Schema   string
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&search_path=%s",
		c.Username, c.Password, c.Host, c.Port, c.Database, c.Schema)
}

type JWTConfig struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type NATSConfig struct {
	URL string
}

type Config struct {
	Port int
	DB   DBConfig
	JWT  JWTConfig
	NATS NATSConfig
}

// Load reads configuration from the environment (and .env, if present).
// Generate the required JWT seed with: go run cmd/genkey/main.go
func Load() (*Config, error) {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return nil, fmt.Errorf("parse PORT: %w", err)
	}

	seedB64 := os.Getenv("JWT_ED25519_SEED_BASE64")
	if seedB64 == "" {
		return nil, fmt.Errorf("JWT_ED25519_SEED_BASE64 is not set (generate with: go run cmd/genkey/main.go)")
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, fmt.Errorf("decode JWT_ED25519_SEED_BASE64: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("JWT_ED25519_SEED_BASE64 must decode to %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	privateKey := ed25519.NewKeyFromSeed(seed)

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	return &Config{
		Port: port,
		DB: DBConfig{
			Host:     os.Getenv("BLUEPRINT_DB_HOST"),
			Port:     os.Getenv("BLUEPRINT_DB_PORT"),
			Database: os.Getenv("BLUEPRINT_DB_DATABASE"),
			Username: os.Getenv("BLUEPRINT_DB_USERNAME"),
			Password: os.Getenv("BLUEPRINT_DB_PASSWORD"),
			Schema:   os.Getenv("BLUEPRINT_DB_SCHEMA"),
		},
		JWT: JWTConfig{
			PrivateKey: privateKey,
			PublicKey:  privateKey.Public().(ed25519.PublicKey),
			AccessTTL:  15 * time.Minute,
			RefreshTTL: 30 * 24 * time.Hour,
		},
		NATS: NATSConfig{URL: natsURL},
	}, nil
}
