package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
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

type RedisConfig struct {
	URL string
}

type PushConfig struct {
	VAPIDPublicKey     string
	VAPIDPrivateKey    string
	VAPIDSubject       string
	FCMProjectID       string
	FCMCredentialsJSON string
}

type MediaConfig struct {
	NodeID        string
	GRPCAddr      string
	WSAddr        string
	RPCToken      string
	RoomCapacity  int
	MaxRoomPeers  int
	JoinTicketTTL time.Duration
}

type TURNConfig struct {
	URL           string
	TLSURL        string
	Secret        string
	CredentialTTL time.Duration
}

type CORSConfig struct {
	AllowedOrigins []string
}

// OriginHostPatterns returns each allowed origin with any `scheme://` prefix
// stripped (so `http://localhost:5173` becomes `localhost:5173`) — the form
// coder/websocket's AcceptOptions.OriginPatterns matches against the request
// Host.
func (c CORSConfig) OriginHostPatterns() []string {
	patterns := make([]string, len(c.AllowedOrigins))
	for i, origin := range c.AllowedOrigins {
		if _, host, ok := strings.Cut(origin, "://"); ok {
			patterns[i] = host
		} else {
			patterns[i] = origin
		}
	}
	return patterns
}

// MediaNodeID is this media pod's identity in the node registry and in
// join-ticket claims: the configured MEDIA_NODE_ID, or the pod's hostname
// when unset. Shared by the heartbeat and the signalling server so both
// agree on which node a ticket was minted for.
func MediaNodeID(cfg *Config) string {
	if cfg.Media.NodeID != "" {
		return cfg.Media.NodeID
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "media-unknown"
}

type Config struct {
	Port     int
	DB       DBConfig
	JWT      JWTConfig
	NATS     NATSConfig
	Redis    RedisConfig
	Push     PushConfig
	CORS     CORSConfig
	Media    MediaConfig
	TURN     TURNConfig
	STUNURLs []string
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

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379"
	}

	var origins []string
	for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		origins = []string{"http://localhost:5173"}
	}

	roomCap := 50
	if v := os.Getenv("MEDIA_ROOM_CAPACITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			roomCap = n
		}
	}
	maxPeers := 16
	if v := os.Getenv("MEDIA_MAX_ROOM_PEERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPeers = n
		}
	}
	joinTTL := 60 * time.Second
	if v := os.Getenv("MEDIA_JOIN_TICKET_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			joinTTL = d
		}
	}
	credTTL := time.Hour
	if v := os.Getenv("TURN_CREDENTIAL_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			credTTL = d
		}
	}
	var stun []string
	for _, s := range strings.Split(os.Getenv("STUN_URLS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			stun = append(stun, s)
		}
	}
	if len(stun) == 0 {
		stun = []string{"stun:stun.l.google.com:19302"}
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
		NATS:  NATSConfig{URL: natsURL},
		Redis: RedisConfig{URL: redisURL},
		Push: PushConfig{
			VAPIDPublicKey:     os.Getenv("PUSH_VAPID_PUBLIC_KEY"),
			VAPIDPrivateKey:    os.Getenv("PUSH_VAPID_PRIVATE_KEY"),
			VAPIDSubject:       os.Getenv("PUSH_VAPID_SUBJECT"),
			FCMProjectID:       os.Getenv("PUSH_FCM_PROJECT_ID"),
			FCMCredentialsJSON: os.Getenv("PUSH_FCM_CREDENTIALS_JSON"),
		},
		CORS: CORSConfig{AllowedOrigins: origins},
		Media: MediaConfig{
			NodeID:        os.Getenv("MEDIA_NODE_ID"),
			GRPCAddr:      os.Getenv("MEDIA_GRPC_ADDR"),
			WSAddr:        os.Getenv("MEDIA_WS_ADDR"),
			RPCToken:      os.Getenv("MEDIA_RPC_TOKEN"),
			RoomCapacity:  roomCap,
			MaxRoomPeers:  maxPeers,
			JoinTicketTTL: joinTTL,
		},
		TURN: TURNConfig{
			URL:           os.Getenv("TURN_URL"),
			TLSURL:        os.Getenv("TURN_TLS_URL"),
			Secret:        os.Getenv("TURN_SECRET"),
			CredentialTTL: credTTL,
		},
		STUNURLs: stun,
	}, nil
}
