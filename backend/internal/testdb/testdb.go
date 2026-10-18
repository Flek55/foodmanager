// Package testdb gives each test its own freshly migrated Postgres database.
//
// It connects to TEST_DATABASE_URL (default: the dev Postgres from
// docker-compose.yml), creates a uniquely named database, applies migrations
// and drops it when the test ends. Without a reachable Postgres the test is
// skipped locally but fails in CI (CI env var set), so it can't silently pass.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/db"
)

const defaultURL = "postgres://foodmanager:foodmanager@localhost:5433/foodmanager?sslmode=disable"

func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = defaultURL
	}
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("connect to test postgres: %v", err)
		}
		t.Skipf("postgres unavailable (run `make db-up`): %v", err)
	}

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "fm_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
		_ = admin.Close(ctx)
	})

	if err := db.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return pool
}
