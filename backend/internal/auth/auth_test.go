package auth

import (
	"context"
	"testing"
	"time"

	"foodmanager/internal/apperr"
	"foodmanager/internal/testdb"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword("correct horse", hash); err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword("wrong horse", hash); ok {
		t.Fatal("wrong password accepted")
	}
	other, _ := HashPassword("correct horse")
	if other == hash {
		t.Fatal("hashes must be salted")
	}
	if _, err := VerifyPassword("x", "not-a-hash"); err == nil {
		t.Fatal("malformed hash must error")
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]bool{
		"  Masha@Example.COM ":  true,
		"a@b.ru":                true,
		"no-at-sign":            false,
		"a@localhost":           false,
		"Masha <m@example.com>": false,
		"":                      false,
	}
	for in, valid := range cases {
		got, err := normalizeEmail(in)
		if valid && err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
		}
		if !valid && err == nil {
			t.Errorf("%q: expected error, got %q", in, got)
		}
	}
	if got, _ := normalizeEmail("  Masha@Example.COM "); got != "masha@example.com" {
		t.Errorf("got %q", got)
	}
}

func TestRegisterLoginLogout(t *testing.T) {
	ctx := context.Background()
	svc := NewService(testdb.New(t))

	user, sess, err := svc.Register(ctx, "Masha@Example.com", "password123", "  Маша  ", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "Маша" || *user.Email != "masha@example.com" {
		t.Fatalf("unexpected user %+v", user)
	}

	_, _, err = svc.Register(ctx, "MASHA@example.com", "password123", "Другая", "")
	assertKind(t, err, apperr.KindConflict)

	got, _, err := svc.Authenticate(ctx, sess.Token)
	if err != nil || got.ID != user.ID {
		t.Fatalf("authenticate: user=%v err=%v", got.ID, err)
	}

	_, _, err = svc.Login(ctx, "masha@example.com", "wrong-password", "")
	assertKind(t, err, apperr.KindUnauthorized)
	_, _, err = svc.Login(ctx, "nobody@example.com", "password123", "")
	assertKind(t, err, apperr.KindUnauthorized)

	_, sess2, err := svc.Login(ctx, " masha@EXAMPLE.com", "password123", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Logout(ctx, sess.Token); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Authenticate(ctx, sess.Token)
	assertKind(t, err, apperr.KindUnauthorized)
	if _, _, err := svc.Authenticate(ctx, sess2.Token); err != nil {
		t.Fatalf("other session must survive logout: %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc := NewService(testdb.New(t))
	ctx := context.Background()

	_, _, err := svc.Register(ctx, "a@b.ru", "short", "Маша", "")
	assertField(t, err, "password")
	_, _, err = svc.Register(ctx, "a@b.ru", "password123", "   ", "")
	assertField(t, err, "displayName")
	_, _, err = svc.Register(ctx, "bad", "password123", "Маша", "")
	assertField(t, err, "email")
}

func TestSessionSlidingRefresh(t *testing.T) {
	ctx := context.Background()
	svc := NewService(testdb.New(t))
	_, sess, err := svc.Register(ctx, "a@b.ru", "password123", "Маша", "")
	if err != nil {
		t.Fatal(err)
	}

	_, refreshed, err := svc.Authenticate(ctx, sess.Token)
	if err != nil || refreshed != nil {
		t.Fatalf("fresh session must not be refreshed: %v %v", refreshed, err)
	}

	svc.now = func() time.Time { return time.Now().Add(sessionRefreshAfter + time.Minute) }
	_, refreshed, err = svc.Authenticate(ctx, sess.Token)
	if err != nil || refreshed == nil {
		t.Fatalf("stale session must be refreshed: %v %v", refreshed, err)
	}
	if !refreshed.ExpiresAt.After(sess.ExpiresAt) {
		t.Fatal("expiry must move forward")
	}
}

func assertKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != kind {
		t.Fatalf("want apperr kind %d, got %v", kind, err)
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != apperr.KindInvalid || e.Field != field {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
}
