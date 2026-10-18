package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"foodmanager/internal/apperr"
	"foodmanager/internal/store"
)

// Telegram identities: linking an account to Telegram (for the bot and
// notifications), logging in through the bot, and logging in from the
// Mini App. A Telegram account belongs to at most one user.
//
// Deep-link tokens (t.me/<bot>?start=<token>) are one-time, short-lived,
// and stored hashed like session tokens.

const (
	LinkTokenPrefix  = "link_"
	LoginTokenPrefix = "login_"

	linkTokenTTL  = 15 * time.Minute
	loginTokenTTL = 5 * time.Minute
)

// TelegramUser is a Telegram user as vouched for by Telegram itself (a bot
// update or signed Mini App init data) — never by the client.
type TelegramUser struct {
	ID        int64
	ChatID    int64
	Username  string
	FirstName string
	LastName  string
}

func (u TelegramUser) displayName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	if name == "" {
		name = "Пользователь Telegram"
	}
	if r := []rune(name); len(r) > maxDisplayNameLen {
		name = string(r[:maxDisplayNameLen])
	}
	return name
}

var (
	errTelegramTaken    = apperr.Conflict("telegram_taken", "этот Telegram уже привязан к другому аккаунту")
	errTokenInvalid     = apperr.NotFoundCode("token_invalid", "ссылка устарела — запросите новую в приложении")
	errTelegramNotFound = apperr.NotFoundCode("telegram_not_linked", "Telegram не привязан к аккаунту")
)

// ---- Linking ----

// TelegramAccount returns the user's linked Telegram account, or nil.
func (s *Service) TelegramAccount(ctx context.Context, userID uuid.UUID) (*store.TelegramAccount, error) {
	acc, err := s.q.GetTelegramAccount(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// NewTelegramLinkToken creates the token for "Подключить Telegram".
func (s *Service) NewTelegramLinkToken(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	return s.newTelegramToken(ctx, LinkTokenPrefix, "link", &userID, "", linkTokenTTL)
}

// LinkTelegramByToken is called by the bot when the user opens the link.
func (s *Service) LinkTelegramByToken(ctx context.Context, token string, tg TelegramUser) (store.User, error) {
	var user store.User
	err := s.inTx(ctx, func(q *store.Queries) error {
		t, err := lockToken(ctx, q, token, "link", s.now())
		if err != nil {
			return err
		}
		if t.UserID == nil {
			return errTokenInvalid
		}
		if err := linkAccount(ctx, q, *t.UserID, tg); err != nil {
			return err
		}
		if err := q.UseTelegramToken(ctx, t.TokenHash); err != nil {
			return err
		}
		user, err = q.GetUserByID(ctx, *t.UserID)
		return err
	})
	return user, err
}

// LinkTelegram links the logged-in user to a Telegram account proven by
// Mini App init data.
func (s *Service) LinkTelegram(ctx context.Context, userID uuid.UUID, tg TelegramUser) (store.TelegramAccount, error) {
	err := s.inTx(ctx, func(q *store.Queries) error { return linkAccount(ctx, q, userID, tg) })
	if err != nil {
		return store.TelegramAccount{}, err
	}
	return s.q.GetTelegramAccount(ctx, userID)
}

func linkAccount(ctx context.Context, q *store.Queries, userID uuid.UUID, tg TelegramUser) error {
	other, err := q.GetTelegramAccountByTelegramID(ctx, tg.ID)
	if err == nil && other.UserID != userID {
		return errTelegramTaken
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = q.UpsertTelegramAccount(ctx, store.UpsertTelegramAccountParams{
		UserID: userID, TelegramUserID: tg.ID, ChatID: tg.ChatID,
		Username: optional(tg.Username), FirstName: optional(tg.FirstName),
	})
	if isUniqueViolation(err) {
		return errTelegramTaken // lost a race with another link of the same Telegram
	}
	return err
}

func (s *Service) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	n, err := s.q.DeleteTelegramAccount(ctx, userID)
	if err == nil && n == 0 {
		return errTelegramNotFound
	}
	return err
}

func (s *Service) UpdateTelegramSettings(ctx context.Context, userID uuid.UUID, notifyAdded, notifyStock bool) (store.TelegramAccount, error) {
	acc, err := s.q.UpdateTelegramSettings(ctx, store.UpdateTelegramSettingsParams{
		NotifyAdded: notifyAdded, NotifyStock: notifyStock, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return acc, errTelegramNotFound
	}
	return acc, err
}

// UserByTelegram finds the user linked to a Telegram account (for the bot).
func (s *Service) UserByTelegram(ctx context.Context, telegramID int64) (store.User, store.TelegramAccount, error) {
	acc, err := s.q.GetTelegramAccountByTelegramID(ctx, telegramID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, acc, errTelegramNotFound
	}
	if err != nil {
		return store.User{}, acc, err
	}
	user, err := s.q.GetUserByID(ctx, acc.UserID)
	return user, acc, err
}

// ---- Login through the bot ----
//
// 1. The login page calls StartTelegramLogin and opens t.me/<bot>?start=login_…
// 2. The bot shows "Войти как Маша?" — ApproveTelegramLogin / RejectTelegramLogin.
//    The explicit confirmation matters: without it, an attacker could send a
//    victim *their own* login link and get logged in as the victim.
// 3. The page polls CompleteTelegramLogin until it gets a session.

type LoginStatus string

const (
	LoginPending  LoginStatus = "pending"
	LoginApproved LoginStatus = "approved"
	LoginExpired  LoginStatus = "expired"
)

func (s *Service) StartTelegramLogin(ctx context.Context, userAgent string) (string, time.Time, error) {
	return s.newTelegramToken(ctx, LoginTokenPrefix, "login", nil, userAgent, loginTokenTTL)
}

// TelegramLoginRequest returns the device (user agent) asking to log in, so
// the bot can show it in the confirmation.
func (s *Service) TelegramLoginRequest(ctx context.Context, token string) (string, error) {
	var ua string
	err := s.inTx(ctx, func(q *store.Queries) error {
		t, err := lockToken(ctx, q, token, "login", s.now())
		if err != nil {
			return err
		}
		if t.ApprovedAt != nil || t.RejectedAt != nil {
			return errTokenInvalid
		}
		if t.UserAgent != nil {
			ua = *t.UserAgent
		}
		return nil
	})
	return ua, err
}

// ApproveTelegramLogin confirms the login as the Telegram user's account,
// creating one if this Telegram isn't linked yet.
func (s *Service) ApproveTelegramLogin(ctx context.Context, token string, tg TelegramUser) (user store.User, created bool, err error) {
	err = s.inTx(ctx, func(q *store.Queries) error {
		t, err := lockToken(ctx, q, token, "login", s.now())
		if err != nil {
			return err
		}
		if t.ApprovedAt != nil || t.RejectedAt != nil {
			return errTokenInvalid
		}
		user, created, err = userForTelegram(ctx, q, tg, true)
		if err != nil {
			return err
		}
		return q.ApproveTelegramToken(ctx, store.ApproveTelegramTokenParams{
			UserID: &user.ID, TelegramUserID: &tg.ID, TokenHash: t.TokenHash,
		})
	})
	return user, created, err
}

func (s *Service) RejectTelegramLogin(ctx context.Context, token string, tg TelegramUser) error {
	return s.inTx(ctx, func(q *store.Queries) error {
		t, err := lockToken(ctx, q, token, "login", s.now())
		if err != nil {
			return err
		}
		return q.RejectTelegramToken(ctx, store.RejectTelegramTokenParams{TelegramUserID: &tg.ID, TokenHash: t.TokenHash})
	})
}

// CompleteTelegramLogin is polled by the login page. Once the bot approved,
// it opens a session (exactly once per token).
func (s *Service) CompleteTelegramLogin(ctx context.Context, token, userAgent string) (store.User, Session, LoginStatus, error) {
	var user store.User
	status := LoginPending
	err := s.inTx(ctx, func(q *store.Queries) error {
		t, err := lockToken(ctx, q, token, "login", s.now())
		if errors.Is(err, errTokenInvalid) {
			status = LoginExpired
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case t.RejectedAt != nil:
			status = LoginExpired
			return nil
		case t.ApprovedAt == nil || t.UserID == nil:
			return nil // still waiting for the user in Telegram
		}
		if err := q.UseTelegramToken(ctx, t.TokenHash); err != nil {
			return err
		}
		status = LoginApproved
		user, err = q.GetUserByID(ctx, *t.UserID)
		return err
	})
	if err != nil || status != LoginApproved {
		return store.User{}, Session{}, status, err
	}
	sess, err := s.openSession(ctx, user, userAgent)
	return user, sess, status, err
}

// ---- Mini App ----

// LoginWithTelegram logs in the user linked to a Mini App's Telegram user.
// With create=false an unknown Telegram returns telegram_not_linked, so the
// app can first offer "log in to your existing account and link it".
func (s *Service) LoginWithTelegram(ctx context.Context, tg TelegramUser, create bool, userAgent string) (store.User, Session, error) {
	var user store.User
	err := s.inTx(ctx, func(q *store.Queries) error {
		var err error
		user, _, err = userForTelegram(ctx, q, tg, create)
		return err
	})
	if err != nil {
		return store.User{}, Session{}, err
	}
	sess, err := s.openSession(ctx, user, userAgent)
	return user, sess, err
}

// userForTelegram finds the linked user, optionally creating a new account
// (no email, no password — Telegram is its only way in).
func userForTelegram(ctx context.Context, q *store.Queries, tg TelegramUser, create bool) (store.User, bool, error) {
	acc, err := q.GetTelegramAccountByTelegramID(ctx, tg.ID)
	if err == nil {
		// Refresh name/username: people change them.
		if _, err := q.UpsertTelegramAccount(ctx, store.UpsertTelegramAccountParams{
			UserID: acc.UserID, TelegramUserID: tg.ID, ChatID: chatOr(tg, acc.ChatID),
			Username: optional(tg.Username), FirstName: optional(tg.FirstName),
		}); err != nil {
			return store.User{}, false, err
		}
		user, err := q.GetUserByID(ctx, acc.UserID)
		return user, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, err
	}
	if !create {
		return store.User{}, false, errTelegramNotFound
	}
	user, err := q.CreateTelegramUser(ctx, tg.displayName())
	if err != nil {
		return store.User{}, false, err
	}
	if err := linkAccount(ctx, q, user.ID, tg); err != nil {
		return store.User{}, false, err
	}
	return user, true, nil
}

// ---- helpers ----

func (s *Service) newTelegramToken(ctx context.Context, prefix, purpose string, userID *uuid.UUID, userAgent string, ttl time.Duration) (string, time.Time, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	// Telegram start parameters allow only [A-Za-z0-9_-], up to 64 chars:
	// base64url fits exactly.
	token := prefix + base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now().Add(ttl)
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	err := s.q.CreateTelegramToken(ctx, store.CreateTelegramTokenParams{
		TokenHash: hashToken(token), Purpose: purpose, UserID: userID,
		UserAgent: optional(userAgent), ExpiresAt: expires,
	})
	return token, expires, err
}

// lockToken loads a live (not expired, not used) token, locking its row.
func lockToken(ctx context.Context, q *store.Queries, token, purpose string, now time.Time) (store.TelegramToken, error) {
	t, err := q.GetTelegramTokenForUpdate(ctx, store.GetTelegramTokenForUpdateParams{TokenHash: hashToken(token), Purpose: purpose})
	if errors.Is(err, pgx.ErrNoRows) {
		return t, errTokenInvalid
	}
	if err != nil {
		return t, err
	}
	if t.UsedAt != nil || now.After(t.ExpiresAt) {
		return t, errTokenInvalid
	}
	return t, nil
}

func (s *Service) inTx(ctx context.Context, fn func(q *store.Queries) error) error {
	if s.pool == nil {
		return errors.New("auth: no pool configured for transactions")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(s.q.WithTx(tx)) })
}

// PurgeExpiredTelegramTokens deletes long-expired deep-link tokens.
func (s *Service) PurgeExpiredTelegramTokens(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredTelegramTokens(ctx)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func chatOr(tg TelegramUser, fallback int64) int64 {
	if tg.ChatID != 0 {
		return tg.ChatID
	}
	return fallback
}
