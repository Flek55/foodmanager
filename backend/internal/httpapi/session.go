package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"foodmanager/internal/apperr"
	"foodmanager/internal/auth"
	"foodmanager/internal/store"
)

const sessionCookieName = "fm_session"

type cookieConfig struct {
	secure bool
}

func (c cookieConfig) session(s auth.Session) string {
	return (&http.Cookie{
		Name:     sessionCookieName,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.ExpiresAt,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

func (c cookieConfig) cleared() string {
	return (&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSessionToken
	ctxUserAgent
)

// sessionMiddleware resolves the session cookie, if any, and puts the user
// into the request context. It never rejects a request by itself: endpoints
// that need a user call currentUser, so public endpoints work unchanged.
func sessionMiddleware(svc *auth.Service, cookies cookieConfig, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxUserAgent, r.UserAgent())

			if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
				ctx = context.WithValue(ctx, ctxSessionToken, c.Value)
				user, refreshed, err := svc.Authenticate(ctx, c.Value)
				switch {
				case err == nil:
					ctx = context.WithValue(ctx, ctxUser, user)
					if refreshed != nil {
						w.Header().Add("Set-Cookie", cookies.session(*refreshed))
					}
				case isKind(err, apperr.KindUnauthorized):
					// Stale cookie: drop it so the browser stops sending it.
					w.Header().Add("Set-Cookie", cookies.cleared())
				default:
					log.ErrorContext(ctx, "authenticate session", "err", err)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// currentUser returns the authenticated user or an Unauthorized error.
func currentUser(ctx context.Context) (store.User, error) {
	if u, ok := ctx.Value(ctxUser).(store.User); ok {
		return u, nil
	}
	return store.User{}, apperr.Unauthorized("нужно войти")
}

func sessionToken(ctx context.Context) string {
	s, _ := ctx.Value(ctxSessionToken).(string)
	return s
}

func userAgent(ctx context.Context) string {
	s, _ := ctx.Value(ctxUserAgent).(string)
	return s
}

func isKind(err error, k apperr.Kind) bool {
	e, ok := apperr.As(err)
	return ok && e.Kind == k
}
