package config

import (
	"strings"
	"testing"
)

func clearSecrets(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"APP_ENV", "TOKEN_ENCRYPTION_KEY", "MOCK_CONTROL_KEY", "WEBHOOK_KEY",
		"API_KEYS", "WEB_API_KEY", "CORS_ALLOWED_ORIGIN", "SEED_ANALYST_EMAIL", "SEED_ANALYST_PASSWORD",
	} {
		t.Setenv(k, "")
	}
}

func TestProductionRequiresExplicitSecrets(t *testing.T) {
	clearSecrets(t)
	t.Setenv("APP_ENV", "production")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOKEN_ENCRYPTION_KEY") {
		t.Fatalf("esperaba error por TOKEN_ENCRYPTION_KEY, obtuve %v", err)
	}

	t.Setenv("TOKEN_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGIN") {
		t.Fatalf("esperaba error por CORS, obtuve %v", err)
	}

	t.Setenv("CORS_ALLOWED_ORIGIN", "https://app.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MOCK_CONTROL_KEY") {
		t.Fatalf("esperaba error por MOCK_CONTROL_KEY, obtuve %v", err)
	}

	t.Setenv("MOCK_CONTROL_KEY", "real-mock-key")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "WEBHOOK_KEY") {
		t.Fatalf("esperaba error por WEBHOOK_KEY, obtuve %v", err)
	}

	t.Setenv("WEBHOOK_KEY", "real-webhook-key")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "API_KEYS") {
		t.Fatalf("esperaba error por API_KEYS, obtuve %v", err)
	}

	t.Setenv("API_KEYS", "key-a,key-b")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("configuración de producción válida rechazada: %v", err)
	}
	if len(cfg.APIKeys) != 2 || cfg.RateLimitPerMinute < 1 {
		t.Fatalf("config inesperada: %+v", cfg)
	}
	if cfg.WebAPIKey != "key-a" {
		t.Fatalf("la SPA debe recibir la primera API key, obtuve %q", cfg.WebAPIKey)
	}
	if len(cfg.DatabaseShards) == 0 {
		t.Fatal("debe haber al menos un shard")
	}
}

func TestDevelopmentKeepsPrototypeDefaults(t *testing.T) {
	clearSecrets(t)
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AllowedOrigin != "*" || cfg.MockControlKey != "dev-mock-key" {
		t.Fatalf("defaults de prototipo inesperados: %+v", cfg)
	}
	if len(cfg.APIKeys) != 0 {
		t.Fatal("en desarrollo API_KEYS puede estar vacío")
	}
}
