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

// ipLimiter is an in-memory per-IP token bucket. Good enough for a single
// instance; would move to Redis/Postgres if we ever run several.
type ipLimiter struct {
	mu      sync.Mutex
	clients map[string]*limiterEntry
	limit   rate.Limit
	burst   int
}

type limiterEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func newIPLimiter(perMinute, burst int) *ipLimiter {
	l := &ipLimiter{
		clients: make(map[string]*limiterEntry),
		limit:   rate.Limit(float64(perMinute) / 60),
		burst:   burst,
	}
	go l.cleanupLoop()
	return l
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.clients[ip]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.clients[ip] = e
	}
	e.lastSeen = time.Now()
	return e.lim.Allow()
}

func (l *ipLimiter) cleanupLoop() {
	for range time.Tick(5 * time.Minute) {
		l.mu.Lock()
		for ip, e := range l.clients {
			if time.Since(e.lastSeen) > 10*time.Minute {
				delete(l.clients, ip)
			}
		}
		l.mu.Unlock()
	}
}

// rateLimitSensitive throttles endpoints open to guessing: password login,
// registration and invite codes. r.RemoteAddr is already the client IP
// thanks to middleware.RealIP (in prod we sit behind Caddy).
func rateLimitSensitive(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			sensitive := (r.Method == http.MethodPost && (p == "/api/auth/login" || p == "/api/auth/register")) ||
				strings.HasPrefix(p, "/api/invites/")
			if sensitive && !l.allow(clientIP(r)) {
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
