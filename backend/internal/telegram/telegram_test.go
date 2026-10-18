package telegram_test

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"foodmanager/internal/telegram"
	"foodmanager/internal/telegram/telegramtest"
)

func TestValidateInitData(t *testing.T) {
	now := time.Now()
	values := url.Values{
		"query_id":  {"AAHdF6IQAAAAAN0XohDhrOrc"},
		"user":      {`{"id":279058397,"first_name":"Маша","username":"masha"}`},
		"auth_date": {strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)},
	}
	signed := telegram.SignInitData(values, telegramtest.Token)

	u, err := telegram.ValidateInitData(signed, telegramtest.Token, time.Hour, now)
	if err != nil || u.ID != 279058397 || u.FirstName != "Маша" || u.Username != "masha" {
		t.Fatalf("valid data rejected: %+v %v", u, err)
	}

	// Signed by another bot.
	if _, err := telegram.ValidateInitData(signed, "999:OTHER", time.Hour, now); err == nil {
		t.Fatal("foreign signature accepted")
	}
	// Tampered: someone swaps the user id.
	tampered := strings.Replace(signed, "279058397", "1", 1)
	if _, err := telegram.ValidateInitData(tampered, telegramtest.Token, time.Hour, now); err == nil {
		t.Fatal("tampered data accepted")
	}
	// Too old: a leaked init string can't be replayed forever.
	if _, err := telegram.ValidateInitData(signed, telegramtest.Token, time.Second, now); err == nil {
		t.Fatal("stale data accepted")
	}
	if _, err := telegram.ValidateInitData("garbage", telegramtest.Token, time.Hour, now); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestClientAgainstFake(t *testing.T) {
	fake := telegramtest.New(t)
	c := telegram.New(telegramtest.Token, fake.URL)
	ctx := context.Background()

	me, err := c.GetMe(ctx)
	if err != nil || me.Username != "test_food_bot" {
		t.Fatalf("getMe: %+v %v", me, err)
	}

	fake.SendText(telegram.User{ID: 42, FirstName: "Маша"}, "привет")
	updates, err := c.GetUpdates(ctx, 0, time.Second)
	if err != nil || len(updates) != 1 || updates[0].Message.Text != "привет" {
		t.Fatalf("getUpdates: %+v %v", updates, err)
	}

	fake.Block(42)
	_, err = c.SendMessage(ctx, telegram.SendMessageParams{ChatID: 42, Text: "hi"})
	if !telegram.IsBlocked(err) {
		t.Fatalf("want blocked error, got %v", err)
	}
}

func TestTokenNeverLeaksIntoErrors(t *testing.T) {
	// Nothing listens here, so the request fails with a transport error
	// that would normally contain the full URL — token included.
	c := telegram.New("123:SECRET-TOKEN", "http://127.0.0.1:1")
	_, err := c.GetMe(context.Background())
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("token leaked or no error: %v", err)
	}
}
