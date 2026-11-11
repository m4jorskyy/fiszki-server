package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host            string
	Port            string
	DatabaseURL     string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	GeminiAPIKey    string
	GeminiModel     string
	AllowedOrigins  []string
}

func Load() Config {
	loadDotEnv(".env")

	return Config{
		Host:            getEnv("HOST", ""),
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://fiszki:fiszki@localhost:5432/fiszki?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		AccessTokenTTL:  time.Duration(getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL: time.Duration(getEnvInt("REFRESH_TOKEN_TTL_DAYS", 30)) * time.Hour * 24,
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		GeminiModel:     getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		AllowedOrigins:  splitAndTrim(getEnv("ALLOWED_ORIGINS", "")),
	}
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			res = append(res, t)
		}
	}
	return res
}

func (c Config) Validate() error {
	secret := strings.TrimSpace(c.JWTSecret)
	if secret == "" ||
		secret == "dev-secret-change-me-in-production" ||
		secret == "change-this-to-a-long-random-secret" ||
		len(secret) < 16 {
		return fmt.Errorf("JWT_SECRET must be set to a strong secret (at least 16 characters) and not a default placeholder")
	}
	return nil
}

func loadDotEnv(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\xef\xbb\xbf")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
