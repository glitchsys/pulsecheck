package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"pulsecheck/internal/config"
)

func validEnv(t *testing.T) {
	t.Helper()
	for _, kv := range [][2]string{
		{"DB_DRIVER", "sqlite"},
		{"SMTP_HOST", ""},
		{"OFFICIAL_STATUS_URL", ""},
		{"ADMIN_PASSWORD", "valid-password"},
		{"SESSION_SECRET", "session-secret-value"},
		{"IP_HASH_PEPPER", "ip-hash-pepper-value"},
		{"SERVICES", "api:API,web:Web App"},
		{"PORT", "8080"},
		{"SPIKE_THRESHOLD_AMBER", ""},
		{"SPIKE_THRESHOLD_RED", ""},
		{"DEDUPE_WINDOW_MINUTES", ""},
		{"BUCKET_MINUTES", ""},
		{"TRUST_PROXY", ""},
		{"COOKIE_SECURE", ""},
		{"PRODUCT_NAME", ""},
		{"SQLITE_PATH", ""},
		{"DATABASE_URL", ""},
	} {
		t.Setenv(kv[0], kv[1])
	}
}

func TestLoadDefaultsAndServices(t *testing.T) {
	validEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != "sqlite" || cfg.ProductName != "PulseCheck" || cfg.BucketMinutes != 15 {
		t.Fatalf("defaults: %+v", cfg)
	}
	if cfg.SQLitePath != "./data/pulsecheck.db" {
		t.Fatal(cfg.SQLitePath)
	}
	if len(cfg.Services) != 2 || cfg.Services[1].Name != "Web App" {
		t.Fatalf("services: %+v", cfg.Services)
	}
	if cfg.Addr() != ":8080" {
		t.Fatal(cfg.Addr())
	}
}

func TestEnvOverridesFile(t *testing.T) {
	validEnv(t)
	path := filepath.Join(t.TempDir(), "pulsecheck.env")
	if err := os.WriteFile(path, []byte("ADMIN_PASSWORD=file-password-value\nPRODUCT_NAME=FromFile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_PASSWORD", "env-password-value")
	if err := os.Unsetenv("PRODUCT_NAME"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminPassword != "env-password-value" {
		t.Fatal(cfg.AdminPassword)
	}
	if cfg.ProductName != "FromFile" {
		t.Fatal(cfg.ProductName)
	}
}

func TestRejectsShortPassword(t *testing.T) {
	validEnv(t)
	t.Setenv("ADMIN_PASSWORD", "short")
	if _, err := config.Load(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectsBadServices(t *testing.T) {
	validEnv(t)
	t.Setenv("SERVICES", "API")
	if _, err := config.Load(""); err == nil {
		t.Fatal("expected error")
	}
}
