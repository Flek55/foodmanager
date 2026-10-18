// Command server runs the Foodmanager backend: HTTP API, and later the
// Telegram bot and background jobs, all in one process.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"foodmanager/internal/auth"
	"foodmanager/internal/config"
	"foodmanager/internal/db"
	"foodmanager/internal/events"
	"foodmanager/internal/household"
	"foodmanager/internal/httpapi"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
)

// version is set at build time: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	// `server -healthcheck` is used by Docker's HEALTHCHECK: the runtime
	// image is distroless (no shell, no curl), so the binary checks itself.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func healthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		return 1
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return 1
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, log); err != nil {
		return err
	}

	hub := events.NewHub()
	authSvc := auth.NewService(store.New(pool))
	handler := httpapi.NewHandler(httpapi.Deps{
		DB:           pool,
		Auth:         authSvc,
		Households:   household.NewService(pool, hub),
		Shopping:     shopping.NewService(pool, hub),
		Hub:          hub,
		SecureCookie: cfg.IsProd(),
		Version:      version,
	})
	go purgeSessionsLoop(ctx, authSvc, log)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(handler, log, httpapi.RouterOptions{StaticDir: cfg.StaticDir, TrustProxy: cfg.TrustProxy}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout on purpose: it would cut SSE streams, which stay
		// open indefinitely. Slow clients are bounded by the proxy instead.
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "version", version, "trust_proxy", cfg.TrustProxy)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func purgeSessionsLoop(ctx context.Context, svc *auth.Service, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := svc.PurgeExpired(ctx)
			if err != nil {
				log.Error("purge expired sessions", "err", err)
			} else if n > 0 {
				log.Info("purged expired sessions", "count", n)
			}
		}
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	if cfg.IsProd() {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
