package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"foodmanager/internal/apperr"
)

// rateLimits protects endpoints open to guessing. Limits are per kind of
// request, so a family onboarding together behind one home IP (5 people ×
// register + view invite + accept) stays well within them, while password
// guessing is capped both per IP and per targeted account.
type rateLimits struct {
	loginPerIP    *limiter
	loginPerEmail *limiter
	register      *limiter
	invites       *limiter // 31^8 possible codes: guessing is hopeless anyway
}

func newRateLimits() *rateLimits {
	return &rateLimits{
		loginPerIP:    newLimiter(20, 10),
		loginPerEmail: newLimiter(5, 5),
		register:      newLimiter(20, 10),
		invites:       newLimiter(60, 30),
	}
}

// limiter is an in-memory token bucket per key. Good enough for a single
// instance; would move to Redis/Postgres if we ever run several.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   rate.Limit
	burst   int
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func newLimiter(perMinute, burst int) *limiter {
	l := &limiter{
		buckets: make(map[string]*bucket),
		limit:   rate.Limit(float64(perMinute) / 60),
		burst:   burst,
	}
	go l.cleanupLoop()
	return l
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = time.Now()
	return b.lim.Allow()
}

func (l *limiter) cleanupLoop() {
	for range time.Tick(5 * time.Minute) {
		l.mu.Lock()
		for key, b := range l.buckets {
			if time.Since(b.lastSeen) > 10*time.Minute {
				delete(l.buckets, key)
			}
		}
		l.mu.Unlock()
	}
}

// rateLimitByIP applies the per-IP limits. Behind a trusted proxy
// r.RemoteAddr is already the real client IP (middleware.RealIP, enabled by
// TRUST_PROXY); the per-email login limit is applied in the Login handler.
func rateLimitByIP(rl *rateLimits) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var l *limiter
			// Not limited on purpose: /api/auth/telegram/complete is polled every
			// couple of seconds and only answers to an unguessable token.
			switch p := r.URL.Path; {
			case r.Method == http.MethodPost && (p == "/api/auth/login" || p == "/api/auth/telegram/webapp"):
				l = rl.loginPerIP
			case r.Method == http.MethodPost && (p == "/api/auth/register" || p == "/api/auth/telegram/start"):
				l = rl.register
			case strings.HasPrefix(p, "/api/invites/"):
				l = rl.invites
			}
			if l != nil && !l.allow(clientIP(r)) {
				writeAppError(w, apperr.TooManyRequests())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP strips the port if present. After middleware.RealIP, RemoteAddr
// may be a bare IP (including IPv6 like "2001:db8::1"), so SplitHostPort
// failing just means there's no port.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
