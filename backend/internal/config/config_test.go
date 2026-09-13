package config

import "testing"

func TestLoadMissingDB(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "test-secret-32-chars-long-enough-123")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoadMissingSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestLoadOK(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://openagent:openagent@localhost:5432/openagent?sslmode=disable")
	t.Setenv("JWT_SECRET", "test-secret-32-chars-long-enough-123")
	t.Setenv("PORT", "8080")
	t.Setenv("ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("port mismatch")
	}
}
