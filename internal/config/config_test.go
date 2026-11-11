package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDotEnv_HandlesBOMQuotesAndComments(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.test")

	// Include a UTF-8 BOM (\xef\xbb\xbf) on the first line, comments, quotes, and blank lines
	content := "\xef\xbb\xbfTEST_PORT=9091\n# comment line\n\nTEST_QUOTED=\"hello-world\"\nTEST_SINGLE='single-val'\nINVALID_LINE_WITHOUT_EQUALS\n"
	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		t.Fatalf("write temp env file: %v", err)
	}

	_ = os.Unsetenv("TEST_PORT")
	_ = os.Unsetenv("TEST_QUOTED")
	_ = os.Unsetenv("TEST_SINGLE")
	t.Cleanup(func() {
		_ = os.Unsetenv("TEST_PORT")
		_ = os.Unsetenv("TEST_QUOTED")
		_ = os.Unsetenv("TEST_SINGLE")
	})

	loadDotEnv(envFile)

	if got := os.Getenv("TEST_PORT"); got != "9091" {
		t.Fatalf("expected TEST_PORT=9091 (with BOM stripped), got %q", got)
	}
	if got := os.Getenv("TEST_QUOTED"); got != "hello-world" {
		t.Fatalf("expected TEST_QUOTED=hello-world, got %q", got)
	}
	if got := os.Getenv("TEST_SINGLE"); got != "single-val" {
		t.Fatalf("expected TEST_SINGLE=single-val, got %q", got)
	}
}

func TestLoad_DefaultsAndEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "8089")
	t.Setenv("ACCESS_TOKEN_TTL_MINUTES", "30")
	t.Setenv("REFRESH_TOKEN_TTL_DAYS", "7")
	t.Setenv("GEMINI_MODEL", "gemini-2.5-pro")

	cfg := Load()

	if cfg.Port != "8089" {
		t.Fatalf("expected Port 8089, got %q", cfg.Port)
	}
	if cfg.AccessTokenTTL != 30*time.Minute {
		t.Fatalf("expected AccessTokenTTL 30m, got %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 7*24*time.Hour {
		t.Fatalf("expected RefreshTokenTTL 7d, got %v", cfg.RefreshTokenTTL)
	}
	if cfg.GeminiModel != "gemini-2.5-pro" {
		t.Fatalf("expected GeminiModel gemini-2.5-pro, got %q", cfg.GeminiModel)
	}
}

func TestGetEnvHelpers_Fallbacks(t *testing.T) {
	_ = os.Unsetenv("NON_EXISTENT_STR")
	_ = os.Unsetenv("NON_EXISTENT_INT")
	t.Setenv("INVALID_INT_VAL", "not-a-number")

	if got := getEnv("NON_EXISTENT_STR", "fallback-str"); got != "fallback-str" {
		t.Fatalf("expected fallback-str, got %q", got)
	}
	if got := getEnvInt("NON_EXISTENT_INT", 42); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := getEnvInt("INVALID_INT_VAL", 99); got != 99 {
		t.Fatalf("expected fallback 99 on invalid int, got %d", got)
	}
}

func TestConfig_Validate(t *testing.T) {
	invalidSecrets := []string{
		"",
		"short-secret",
		"dev-secret-change-me-in-production",
		"change-this-to-a-long-random-secret",
	}
	for _, s := range invalidSecrets {
		cfg := Config{JWTSecret: s}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected Validate() error for weak/placeholder JWT_SECRET %q", s)
		}
	}

	validCfg := Config{JWTSecret: "a-very-strong-production-jwt-secret-key-2026"}
	if err := validCfg.Validate(); err != nil {
		t.Fatalf("expected valid config to pass Validate(), got %v", err)
	}
}
