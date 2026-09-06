package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func TestLoadConfig_StoreTypeDefaultsToRedis(t *testing.T) {
	path := writeConfig(t, `
store:
  redis:
    url: "redis://localhost:6379"
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Store.Type != StoreTypeDefault {
		t.Errorf("expected default store type %q, got %q", StoreTypeDefault, cfg.Store.Type)
	}

	if cfg.Store.Redis.TTL != 300*time.Second {
		t.Errorf("expected default redis ttl 300s, got %v", cfg.Store.Redis.TTL)
	}

	if cfg.Store.Postgres.TTL != 300*time.Second {
		t.Errorf("expected default postgres ttl 300s, got %v", cfg.Store.Postgres.TTL)
	}

	if cfg.Store.Memory.TTL != 300*time.Second {
		t.Errorf("expected default memory ttl 300s, got %v", cfg.Store.Memory.TTL)
	}
}

func TestLoadConfig_FullStoreConfig(t *testing.T) {
	t.Setenv("REDIS_URL", "redis://cache.internal:6379")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@db:5432/e2e")

	path := writeConfig(t, `
store:
  type: postgres
  redis:
    url: "{{env.REDIS_URL}}"
    ttl: 60s
  postgres:
    dsn: "{{env.POSTGRES_DSN}}"
    ttl: 45s
  memory:
    ttl: 10s
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Store.Type != "postgres" {
		t.Errorf("expected type postgres, got %q", cfg.Store.Type)
	}

	if cfg.Store.Postgres.DSN != "postgres://u:p@db:5432/e2e" {
		t.Errorf("expected resolved postgres dsn, got %q", cfg.Store.Postgres.DSN)
	}

	if cfg.Store.Postgres.TTL != 45*time.Second {
		t.Errorf("expected postgres ttl 45s, got %v", cfg.Store.Postgres.TTL)
	}

	if cfg.Store.Redis.URL != "redis://cache.internal:6379" {
		t.Errorf("expected resolved redis url, got %q", cfg.Store.Redis.URL)
	}

	if cfg.Store.Redis.TTL != 60*time.Second {
		t.Errorf("expected redis ttl 60s, got %v", cfg.Store.Redis.TTL)
	}

	if cfg.Store.Memory.TTL != 10*time.Second {
		t.Errorf("expected memory ttl 10s, got %v", cfg.Store.Memory.TTL)
	}
}

func TestLoadConfig_StoreTypeEnvOverride(t *testing.T) {
	path := writeConfig(t, `
store:
  type: redis
  redis:
    url: "redis://localhost:6379"
`)
	t.Setenv("STORE_TYPE", "none")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Store.Type != "none" {
		t.Errorf("expected STORE_TYPE override to none, got %q", cfg.Store.Type)
	}
}
