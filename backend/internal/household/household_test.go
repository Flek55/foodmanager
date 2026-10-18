package household

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
	"foodmanager/internal/store"
	"foodmanager/internal/testdb"
)

func TestInviteCodeFormat(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		code, err := newInviteCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != codeLen {
			t.Fatalf("bad length: %q", code)
		}
		for _, r := range code {
			if !strings.ContainsRune(codeAlphabet, r) {
				t.Fatalf("symbol %q outside alphabet in %q", r, code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 1000 {
		t.Fatal("unexpected duplicate codes")
	}
	if got := NormalizeInviteCode(" k7qm-2xpa "); got != "K7QM2XPA" {
		t.Errorf("normalize: %q", got)
	}
	if got := FormatInviteCode("K7QM2XPA"); got != "K7QM-2XPA" {
		t.Errorf("format: %q", got)
	}
}

type fixture struct {
	ctx  context.Context
	pool *pgxpool.Pool
	svc  *Service
}

func setup(t *testing.T) fixture {
	pool := testdb.New(t)
	return fixture{ctx: context.Background(), pool: pool, svc: NewService(pool, events.Nop{})}
}

func (f fixture) user(t *testing.T, name string) uuid.UUID {
	t.Helper()
	u, err := store.New(f.pool).CreateUser(f.ctx, store.CreateUserParams{DisplayName: name})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestCreateAndAccess(t *testing.T) {
	f := setup(t)
	masha, petya := f.user(t, "Маша"), f.user(t, "Петя")

	d, err := f.svc.Create(f.ctx, masha, "  Ивановы ")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Ивановы" || d.Role != RoleOwner || len(d.Members) != 1 {
		t.Fatalf("unexpected detail %+v", d)
	}

	_, err = f.svc.Get(f.ctx, petya, d.ID)
	assertKind(t, err, apperr.KindNotFound) // outsiders can't even see it exists

	_, err = f.svc.Create(f.ctx, masha, "   ")
	assertKind(t, err, apperr.KindInvalid)
}

func TestInviteFlow(t *testing.T) {
	f := setup(t)
	masha, petya, vasya := f.user(t, "Маша"), f.user(t, "Петя"), f.user(t, "Вася")
	h, _ := f.svc.Create(f.ctx, masha, "Ивановы")

	_, err := f.svc.CreateInvite(f.ctx, petya, h.ID, 1, time.Hour)
	assertKind(t, err, apperr.KindNotFound) // non-member can't create invites

	inv, err := f.svc.CreateInvite(f.ctx, masha, h.ID, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Users type codes in any case and with a dash.
	typed := strings.ToLower(FormatInviteCode(inv.Code))
	p, err := f.svc.PreviewInvite(f.ctx, typed)
	if err != nil || p.HouseholdName != "Ивановы" || p.InvitedBy != "Маша" || p.MemberCount != 1 {
		t.Fatalf("preview: %+v %v", p, err)
	}

	d, err := f.svc.AcceptInvite(f.ctx, petya, typed)
	if err != nil {
		t.Fatal(err)
	}
	if d.Role != RoleMember || len(d.Members) != 2 {
		t.Fatalf("after accept: %+v", d)
	}

	// The single use is gone, so Vasya can't join with it.
	_, err = f.svc.AcceptInvite(f.ctx, vasya, inv.Code)
	assertKind(t, err, apperr.KindNotFound)

	inv2, _ := f.svc.CreateInvite(f.ctx, petya, h.ID, 5, time.Hour)
	if err := f.svc.RevokeInvite(f.ctx, masha, h.ID, FormatInviteCode(inv2.Code)); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AcceptInvite(f.ctx, vasya, inv2.Code)
	assertKind(t, err, apperr.KindNotFound)

	list, err := f.svc.ListInvites(f.ctx, masha, h.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("no active invites expected, got %d (%v)", len(list), err)
	}
}

func TestReacceptDoesNotConsumeUse(t *testing.T) {
	f := setup(t)
	masha, petya := f.user(t, "Маша"), f.user(t, "Петя")
	h, _ := f.svc.Create(f.ctx, masha, "Ивановы")
	inv, _ := f.svc.CreateInvite(f.ctx, masha, h.ID, 2, time.Hour)

	for range 2 {
		if _, err := f.svc.AcceptInvite(f.ctx, petya, inv.Code); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := f.svc.ListInvites(f.ctx, masha, h.ID)
	if len(list) != 1 || list[0].Uses != 1 {
		t.Fatalf("want 1 use, got %+v", list)
	}
}

func TestInviteValidation(t *testing.T) {
	f := setup(t)
	masha := f.user(t, "Маша")
	h, _ := f.svc.Create(f.ctx, masha, "Ивановы")

	_, err := f.svc.CreateInvite(f.ctx, masha, h.ID, 0, time.Hour)
	assertKind(t, err, apperr.KindInvalid)
	_, err = f.svc.CreateInvite(f.ctx, masha, h.ID, 5, 365*24*time.Hour)
	assertKind(t, err, apperr.KindInvalid)
}

func TestRemoveMemberRules(t *testing.T) {
	f := setup(t)
	masha, petya, vasya := f.user(t, "Маша"), f.user(t, "Петя"), f.user(t, "Вася")
	h, _ := f.svc.Create(f.ctx, masha, "Ивановы")
	inv, _ := f.svc.CreateInvite(f.ctx, masha, h.ID, 10, time.Hour)
	_, _ = f.svc.AcceptInvite(f.ctx, petya, inv.Code)
	_, _ = f.svc.AcceptInvite(f.ctx, vasya, inv.Code)

	err := f.svc.RemoveMember(f.ctx, petya, h.ID, vasya)
	assertKind(t, err, apperr.KindForbidden)

	if err := f.svc.RemoveMember(f.ctx, masha, h.ID, vasya); err != nil {
		t.Fatal(err)
	}

	// The last owner leaves: Petya, the oldest remaining member, takes over.
	if err := f.svc.RemoveMember(f.ctx, masha, h.ID, masha); err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.Get(f.ctx, petya, h.ID)
	if err != nil || d.Role != RoleOwner || len(d.Members) != 1 {
		t.Fatalf("petya should be the sole owner: %+v %v", d, err)
	}

	// The last member leaves: the household is gone.
	if err := f.svc.RemoveMember(f.ctx, petya, h.ID, petya); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM households").Scan(&n); err != nil || n != 0 {
		t.Fatalf("household should be deleted, count=%d err=%v", n, err)
	}
}

func assertKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != kind {
		t.Fatalf("want apperr kind %d, got %v", kind, err)
	}
}
