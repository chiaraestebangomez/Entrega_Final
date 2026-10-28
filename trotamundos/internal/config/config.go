package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr string
	Env      string

	DatabaseURL     string
	DatabaseShards  []string
	OLAPDatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTPrivateKeyPath string
	JWTPublicKeyPath  string
	JWTIssuer         string
	JWTAccessTTL      time.Duration
	RefreshTTL        time.Duration

	TokenEncryptionKey []byte

	MockControlKey string
	WebhookKey     string

	EnableWorkers         bool
	IngestionPollInterval time.Duration
	ProviderSyncInterval  time.Duration
	AutoArchiveInterval   time.Duration
	StaleEventAfter       time.Duration

	SeedAnalystEmail    string
	SeedAnalystPassword string

	ShardCount    int
	AllowedOrigin string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		Env:                 env("APP_ENV", "development"),
		DatabaseURL:         env("DATABASE_URL", "postgres://trotamundos:trotamundos@localhost:5432/trotamundos?sslmode=disable"),
		OLAPDatabaseURL:     os.Getenv("OLAP_DATABASE_URL"),
		RedisAddr:           env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		JWTPrivateKeyPath:   env("JWT_PRIVATE_KEY_PATH", "keys/jwt_private.pem"),
		JWTPublicKeyPath:    env("JWT_PUBLIC_KEY_PATH", "keys/jwt_public.pem"),
		JWTIssuer:           env("JWT_ISSUER", "trotamundos"),
		MockControlKey:      env("MOCK_CONTROL_KEY", "dev-mock-key"),
		WebhookKey:          env("WEBHOOK_KEY", "dev-webhook-key"),
		SeedAnalystEmail:    os.Getenv("SEED_ANALYST_EMAIL"),
		SeedAnalystPassword: os.Getenv("SEED_ANALYST_PASSWORD"),
		AllowedOrigin:       env("CORS_ALLOWED_ORIGIN", "*"),
	}

	var err error
	if c.RedisDB, err = envInt("REDIS_DB", 0); err != nil {
		return c, err
	}
	if c.JWTAccessTTL, err = envDuration("JWT_ACCESS_TTL", 15*time.Minute); err != nil {
		return c, err
	}
	if c.RefreshTTL, err = envDuration("REFRESH_TTL", 30*24*time.Hour); err != nil {
		return c, err
	}
	if c.IngestionPollInterval, err = envDuration("INGESTION_POLL_INTERVAL", 2*time.Second); err != nil {
		return c, err
	}
	if c.ProviderSyncInterval, err = envDuration("PROVIDER_SYNC_INTERVAL", 2*time.Minute); err != nil {
		return c, err
	}
	if c.AutoArchiveInterval, err = envDuration("AUTO_ARCHIVE_INTERVAL", 10*time.Minute); err != nil {
		return c, err
	}
	if c.StaleEventAfter, err = envDuration("STALE_EVENT_AFTER", 2*time.Minute); err != nil {
		return c, err
	}
	if c.EnableWorkers, err = envBool("ENABLE_WORKERS", true); err != nil {
		return c, err
	}
	if c.ShardCount, err = envInt("SHARD_COUNT", 1); err != nil {
		return c, err
	}
	if c.ShardCount < 1 {
		return c, fmt.Errorf("SHARD_COUNT must be >= 1")
	}

	shards := strings.TrimSpace(os.Getenv("DATABASE_SHARDS"))
	if shards == "" {
		shards = c.DatabaseURL
	}
	for _, s := range strings.Split(shards, ",") {
		if v := strings.TrimSpace(s); v != "" {
			c.DatabaseShards = append(c.DatabaseShards, v)
		}
	}
	if len(c.DatabaseShards) == 0 {
		return c, fmt.Errorf("no database shards configured")
	}
	if c.OLAPDatabaseURL == "" {
		c.OLAPDatabaseURL = c.DatabaseShards[0]
	}

	key := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if key == "" {
		key = base64.StdEncoding.EncodeToString([]byte("trotamundos-dev-token-key-32byte"))
	}
	c.TokenEncryptionKey, err = base64.StdEncoding.DecodeString(key)
	if err != nil {
		return c, fmt.Errorf("TOKEN_ENCRYPTION_KEY must be base64: %w", err)
	}
	if len(c.TokenEncryptionKey) != 32 {
		return c, fmt.Errorf("TOKEN_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(c.TokenEncryptionKey))
	}

	return c, nil
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}

func envBool(name string, fallback bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return b, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}
