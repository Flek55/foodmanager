package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"foodmanager/internal/api"
	"foodmanager/internal/auth"
	"foodmanager/internal/events"
	"foodmanager/internal/household"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
	"foodmanager/internal/testdb"
)

// client is a tiny browser stand-in: it keeps cookies between requests.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newServer(t *testing.T) string {
	pool := testdb.New(t)
	hub := events.NewHub()
	h := NewHandler(Deps{
		DB:         pool,
		Auth:       auth.NewService(store.New(pool)),
		Households: household.NewService(pool, hub),
		Shopping:   shopping.NewService(pool, hub),
		Hub:        hub,
		Version:    "test",
	})
	srv := httptest.NewServer(NewRouter(h, slog.New(slog.DiscardHandler), RouterOptions{}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+"/api"+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func TestFamilyOnboardingFlow(t *testing.T) {
	base := newServer(t)
	masha, petya := newClient(t, base), newClient(t, base)

	if code := masha.do("GET", "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous /me: %d", code)
	}

	var me api.Me
	if code := masha.do("POST", "/auth/register", api.RegisterRequest{Email: "masha@example.com", Password: "password123", DisplayName: "Маша"}, &me); code != http.StatusCreated {
		t.Fatalf("register: %d", code)
	}
	if code := masha.do("GET", "/me", nil, &me); code != http.StatusOK || me.User.DisplayName != "Маша" {
		t.Fatalf("/me after register: %d %+v", code, me)
	}

	var h api.HouseholdDetail
	if code := masha.do("POST", "/households", api.HouseholdInput{Name: "Ивановы"}, &h); code != http.StatusCreated {
		t.Fatalf("create household: %d", code)
	}
	var inv api.Invite
	if code := masha.do("POST", "/households/"+h.Id.String()+"/invites", api.InviteInput{}, &inv); code != http.StatusCreated {
		t.Fatalf("create invite: %d", code)
	}
	if len(inv.Code) != 9 || inv.Code[4] != '-' || inv.MaxUses != household.DefaultInviteUses {
		t.Fatalf("unexpected invite %+v", inv)
	}

	var preview api.InvitePreview
	if code := petya.do("GET", "/invites/"+inv.Code, nil, &preview); code != http.StatusOK || preview.HouseholdName != "Ивановы" {
		t.Fatalf("anonymous preview: %d %+v", code, preview)
	}
	if code := petya.do("POST", "/invites/"+inv.Code+"/accept", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous accept: %d", code)
	}

	petya.do("POST", "/auth/register", api.RegisterRequest{Email: "petya@example.com", Password: "password123", DisplayName: "Петя"}, nil)
	var joined api.HouseholdDetail
	if code := petya.do("POST", "/invites/"+inv.Code+"/accept", nil, &joined); code != http.StatusOK || len(joined.Members) != 2 || joined.Role != api.RoleMember {
		t.Fatalf("accept: %d %+v", code, joined)
	}

	// Petya isn't the owner, so he can't rename the household.
	if code := petya.do("PATCH", "/households/"+h.Id.String(), api.HouseholdInput{Name: "Петровы"}, nil); code != http.StatusForbidden {
		t.Fatalf("member rename: %d", code)
	}

	if code := petya.do("POST", "/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	if code := petya.do("GET", "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("/me after logout: %d", code)
	}
}

func TestErrorFormat(t *testing.T) {
	c := newClient(t, newServer(t))
	req, _ := http.NewRequest("POST", c.base+"/api/auth/register", bytes.NewBufferString(`{"email":"bad","password":"password123","displayName":"Маша"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var e api.Error
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != http.StatusBadRequest || e.Code != "invalid" || e.Field == nil || *e.Field != "email" {
		t.Fatalf("got %d %+v", resp.StatusCode, e)
	}
}

func TestCrossSiteRequestRejected(t *testing.T) {
	c := newClient(t, newServer(t))
	req, _ := http.NewRequest("POST", c.base+"/api/auth/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site POST must be rejected, got %d", resp.StatusCode)
	}
}

// Without TRUST_PROXY, a client can't dodge the per-IP limiter by inventing
// a new IP for every attempt. (Different emails, so the per-account limit
// doesn't kick in first.)
func TestSpoofedIPHeadersIgnoredByDefault(t *testing.T) {
	c := newClient(t, newServer(t))
	last := 0
	for i := range 11 {
		req, _ := http.NewRequest("POST", c.base+"/api/auth/login",
			bytes.NewBufferString(fmt.Sprintf(`{"email":"x%d@example.com","password":"password123"}`, i)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", fmt.Sprintf("10.0.0.%d", i))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.1.%d", i))
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("spoofed IP headers bypassed the rate limit: last status %d", last)
	}
}

// Guessing one account's password is capped well before the per-IP limit.
func TestLoginRateLimitedPerAccount(t *testing.T) {
	c := newClient(t, newServer(t))
	body := api.LoginRequest{Email: "x@example.com", Password: "wrong-password"}
	codes := make([]int, 6)
	for i := range codes {
		codes[i] = c.do("POST", "/auth/login", body, nil)
	}
	if codes[4] != http.StatusUnauthorized || codes[5] != http.StatusTooManyRequests {
		t.Fatalf("want 5×401 then 429, got %v", codes)
	}
	// Another account from the same IP is still fine.
	if code := c.do("POST", "/auth/login", api.LoginRequest{Email: "y@example.com", Password: "x"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("other account: %d", code)
	}
}

// Five relatives onboarding at the same table (one home IP) must not hit
// the limits: register, look at the invite, accept.
func TestFamilyOnboardingFitsRateLimits(t *testing.T) {
	base := newServer(t)
	owner, hid := family(t, base, "owner@example.com")
	var inv api.Invite
	owner.do("POST", "/households/"+hid+"/invites", api.InviteInput{}, &inv)

	for i := range 5 {
		c := newClient(t, base)
		reg := api.RegisterRequest{Email: fmt.Sprintf("relative%d@example.com", i), Password: "password123", DisplayName: "Родственник"}
		if code := c.do("POST", "/auth/register", reg, nil); code != http.StatusCreated {
			t.Fatalf("relative %d register: %d", i, code)
		}
		for range 2 { // the join page may load the preview more than once
			if code := c.do("GET", "/invites/"+inv.Code, nil, nil); code != http.StatusOK {
				t.Fatalf("relative %d preview: %d", i, code)
			}
		}
		if code := c.do("POST", "/invites/"+inv.Code+"/accept", nil, nil); code != http.StatusOK {
			t.Fatalf("relative %d accept: %d", i, code)
		}
	}
}
