package httpapi

import (
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
)

// NewRouter wires middleware, the generated API routes under /api and,
// optionally, the built SPA from staticDir.
func NewRouter(h *Handler, log *slog.Logger, staticDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)
	// Rejects cross-site state-changing requests (CSRF) using the
	// Sec-Fetch-Site / Origin headers that all modern browsers send.
	r.Use(http.NewCrossOriginProtection().Handler)

	session := sessionMiddleware(h.auth, h.cookies, log)

	strict := api.NewStrictHandlerWithOptions(h, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeAppError(w, apperr.Invalid("", "некорректный запрос: "+err.Error()))
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if _, ok := apperr.As(err); !ok {
				log.ErrorContext(r.Context(), "handler failed", "err", err, "path", r.URL.Path)
			}
			writeAppError(w, err)
		},
	})
	api.HandlerWithOptions(strict, api.ChiServerOptions{
		BaseURL:    "/api",
		BaseRouter: r,
		// Applied per API route; the LAST entry is the outermost, so the rate
		// limiter runs before the session lookup hits the database.
		Middlewares: []api.MiddlewareFunc{
			session,
			rateLimitSensitive(newIPLimiter(10, 10)),
		},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeAppError(w, apperr.Invalid("", "некорректный запрос: "+err.Error()))
		},
	})
	r.With(session).Get("/api/households/{householdId}/events", h.serveEvents)
	r.HandleFunc("/api/*", func(w http.ResponseWriter, _ *http.Request) {
		writeAppError(w, apperr.NotFound("эндпоинт не найден"))
	})

	if staticDir != "" {
		r.Handle("/*", spaHandler(staticDir))
	}
	return r
}

// writeAppError maps domain errors to HTTP statuses; anything else is a 500
// with a generic message (details go to the log, not to the client).
func writeAppError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	body := api.Error{Code: "internal", Message: "внутренняя ошибка сервера"}

	if e, ok := apperr.As(err); ok {
		switch e.Kind {
		case apperr.KindInvalid:
			status = http.StatusBadRequest
		case apperr.KindUnauthorized:
			status = http.StatusUnauthorized
		case apperr.KindForbidden:
			status = http.StatusForbidden
		case apperr.KindNotFound:
			status = http.StatusNotFound
		case apperr.KindConflict:
			status = http.StatusConflict
		case apperr.KindTooManyRequests:
			status = http.StatusTooManyRequests
		}
		body = api.Error{Code: e.Code, Message: e.Message}
		if e.Field != "" {
			body.Field = &e.Field
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func init() {
	// Not in Go's built-in table; browsers want this type for PWA manifests.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// spaHandler serves files from dir and falls back to index.html for unknown
// paths, so client-side routes like /list/123 survive a page reload.
//
// Caching: /assets/* have content hashes in their names (Vite), so they are
// cached forever; everything else (index.html, sw.js, manifest, icons) is
// revalidated on every load, so new deploys reach users immediately.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
