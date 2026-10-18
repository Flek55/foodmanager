// Package config loads application settings from environment variables.
// Defaults are tuned for local development, so `go run ./cmd/server` works
// out of the box against the dev Postgres from docker-compose.yml.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	// Env is "dev" or "prod". Affects log format and cookie security.
	Env string
	// HTTPAddr is the listen address of the HTTP server.
	HTTPAddr string
	// DatabaseURL is a pgx connection string.
	DatabaseURL string
	// StaticDir, if set, is served as the SPA (built frontend). Empty in dev,
	// where Vite serves the frontend and proxies /api to us.
	StaticDir string
	// TrustProxy: take the client IP from X-Real-IP / X-Forwarded-For.
	// Enable ONLY behind a reverse proxy that sets these headers (Caddy in
	// prod); otherwise any client could fake its IP to dodge rate limits.
	TrustProxy bool
	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Env:             getenv("APP_ENV", "dev"),
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://foodmanager:foodmanager@localhost:5433/foodmanager?sslmode=disable"),
		StaticDir:       os.Getenv("STATIC_DIR"),
		TrustProxy:      os.Getenv("TRUST_PROXY") == "true",
		ShutdownTimeout: 10 * time.Second,
	}
	if cfg.Env != "dev" && cfg.Env != "prod" {
		return Config{}, fmt.Errorf("APP_ENV must be dev or prod, got %q", cfg.Env)
	}
	return cfg, nil
}

func (c Config) IsProd() bool { return c.Env == "prod" }

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
