// Package auth handles accounts, password login and server-side sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/apperr"
	"foodmanager/internal/store"
)

const (
	// SessionTTL is how long a session lives without activity.
	SessionTTL = 30 * 24 * time.Hour
	// sessionRefreshAfter limits writes: an active session is extended at
	// most once per this interval instead of on every request.
	sessionRefreshAfter = 12 * time.Hour
	maxUserAgentLen     = 256
)

type Service struct {
	pool *pgxpool.Pool
	q    *store.Queries
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: store.New(pool), now: time.Now}
}

// Session is an opened session: the raw token goes into the cookie and is
// never stored server-side.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

func (s *Service) Register(ctx context.Context, email, password, displayName, userAgent string) (store.User, Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return store.User{}, Session{}, err
	}
	if err := validatePassword(password); err != nil {
		return store.User{}, Session{}, err
	}
	displayName, err = normalizeDisplayName(displayName)
	if err != nil {
		return store.User{}, Session{}, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, Session{}, err
	}
	user, err := s.q.CreateUser(ctx, store.CreateUserParams{
		DisplayName:  displayName,
		Email:        &email,
		PasswordHash: &hash,
	})
	if isUniqueViolation(err) {
		return store.User{}, Session{}, apperr.Conflict("email_taken", "этот email уже зарегистрирован, попробуйте войти")
	}
	if err != nil {
		return store.User{}, Session{}, err
	}

	sess, err := s.openSession(ctx, user, userAgent)
	return user, sess, err
}

func (s *Service) Login(ctx context.Context, email, password, userAgent string) (store.User, Session, error) {
	errBadCredentials := apperr.Unauthorized("неверный email или пароль")

	email, err := normalizeEmail(email)
	if err != nil {
		return store.User{}, Session{}, errBadCredentials
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, Session{}, err
	}

	hash := dummyHash
	if err == nil && user.PasswordHash != nil {
		hash = *user.PasswordHash
	}
	ok, verr := VerifyPassword(password, hash)
	if verr != nil {
		return store.User{}, Session{}, verr
	}
	if err != nil || user.PasswordHash == nil || !ok {
		return store.User{}, Session{}, errBadCredentials
	}

	sess, err := s.openSession(ctx, user, userAgent)
	return user, sess, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.q.DeleteSession(ctx, hashToken(token))
}

// Authenticate resolves a session token to its user. If the session was
// extended, refreshed holds the new expiry so the caller can renew the cookie.
func (s *Service) Authenticate(ctx context.Context, token string) (user store.User, refreshed *Session, err error) {
	if token == "" {
		return store.User{}, nil, apperr.Unauthorized("нужно войти")
	}
	row, err := s.q.GetActiveSession(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, nil, apperr.Unauthorized("сессия истекла, войдите снова")
	}
	if err != nil {
		return store.User{}, nil, err
	}

	now := s.now()
	if now.Sub(row.LastSeenAt) >= sessionRefreshAfter {
		expires := now.Add(SessionTTL)
		if err := s.q.ExtendSession(ctx, store.ExtendSessionParams{TokenHash: hashToken(token), ExpiresAt: expires}); err != nil {
			return store.User{}, nil, err
		}
		refreshed = &Session{Token: token, ExpiresAt: expires}
	}
	return row.User, refreshed, nil
}

// PurgeExpired deletes expired sessions; meant for a periodic background job.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredSessions(ctx)
}

func (s *Service) openSession(ctx context.Context, user store.User, userAgent string) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now().Add(SessionTTL)

	var ua *string
	if userAgent != "" {
		if len(userAgent) > maxUserAgentLen {
			userAgent = userAgent[:maxUserAgentLen]
		}
		ua = &userAgent
	}
	err := s.q.CreateSession(ctx, store.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    user.ID,
		UserAgent: ua,
		ExpiresAt: expires,
	})
	return Session{Token: token, ExpiresAt: expires}, err
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
