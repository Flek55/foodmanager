package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"foodmanager/internal/api"
	"foodmanager/internal/auth"
	"foodmanager/internal/telegram"
	"foodmanager/internal/telegram/telegramtest"
)

var tgConfig = &TelegramConfig{BotToken: telegramtest.Token, BotUsername: "test_food_bot"}

// initData builds Mini App init data signed like Telegram does.
func initData(tgID int64, firstName string) string {
	return telegram.SignInitData(url.Values{
		"user":      {`{"id":` + strconv.FormatInt(tgID, 10) + `,"first_name":"` + firstName + `"}`},
		"auth_date": {strconv.FormatInt(time.Now().Unix(), 10)},
	}, telegramtest.Token)
}

func TestConfigReportsTelegram(t *testing.T) {
	var cfg api.AppConfig
	newClient(t, newServer(t)).do("GET", "/config", nil, &cfg)
	if cfg.TelegramBot != nil {
		t.Fatalf("no bot configured, got %q", *cfg.TelegramBot)
	}
	base, _ := newServerWith(t, tgConfig)
	newClient(t, base).do("GET", "/config", nil, &cfg)
	if cfg.TelegramBot == nil || *cfg.TelegramBot != "test_food_bot" {
		t.Fatalf("bot not reported: %+v", cfg)
	}
}

func TestMiniAppLogin(t *testing.T) {
	base, _ := newServerWith(t, tgConfig)
	c := newClient(t, base)
	data := initData(5005, "Вася")

	// Unknown Telegram: the app should offer "log in and link" first.
	req := api.WebAppLoginRequest{InitData: data}
	if code := c.do("POST", "/auth/telegram/webapp", req, nil); code != http.StatusNotFound {
		t.Fatalf("unlinked: %d", code)
	}

	create := true
	req.Create = &create
	var me api.Me
	if code := c.do("POST", "/auth/telegram/webapp", req, &me); code != http.StatusOK {
		t.Fatalf("create: %d", code)
	}
	if me.User.DisplayName != "Вася" || me.Telegram == nil {
		t.Fatalf("new account: %+v", me)
	}
	if code := c.do("GET", "/me", nil, nil); code != http.StatusOK {
		t.Fatalf("session not set: %d", code)
	}

	// The same Telegram later logs straight into that account.
	var again api.Me
	newClient(t, base).do("POST", "/auth/telegram/webapp", api.WebAppLoginRequest{InitData: initData(5005, "Вася")}, &again)
	if again.User.Id != me.User.Id {
		t.Fatal("second login landed in another account")
	}
}

func TestMiniAppRejectsForgedData(t *testing.T) {
	base, _ := newServerWith(t, tgConfig)
	c := newClient(t, base)

	forged := strings.Replace(initData(5005, "Вася"), "5005", "1", 1)
	if code := c.do("POST", "/auth/telegram/webapp", api.WebAppLoginRequest{InitData: forged}, nil); code != http.StatusUnauthorized {
		t.Fatalf("forged user id: %d", code)
	}
	otherBot := telegram.SignInitData(url.Values{
		"user": {`{"id":5005,"first_name":"Вася"}`}, "auth_date": {strconv.FormatInt(time.Now().Unix(), 10)},
	}, "999:OTHER-BOT")
	if code := c.do("POST", "/auth/telegram/webapp", api.WebAppLoginRequest{InitData: otherBot}, nil); code != http.StatusUnauthorized {
		t.Fatalf("other bot's data: %d", code)
	}
}

func TestLinkFromMiniAppAndSettings(t *testing.T) {
	base, _ := newServerWith(t, tgConfig)
	masha := newClient(t, base)
	masha.do("POST", "/auth/register", api.RegisterRequest{Email: "masha@example.com", Password: "password123", DisplayName: "Маша"}, nil)

	var acc api.TelegramAccount
	if code := masha.do("POST", "/telegram/link/webapp", api.WebAppInitData{InitData: initData(6006, "Маша")}, &acc); code != http.StatusOK {
		t.Fatalf("link: %d", code)
	}
	if !acc.NotifyAdded || !acc.NotifyStock {
		t.Fatalf("notifications should default to on: %+v", acc)
	}

	// Someone else can't take over that Telegram.
	petya := newClient(t, base)
	petya.do("POST", "/auth/register", api.RegisterRequest{Email: "petya@example.com", Password: "password123", DisplayName: "Петя"}, nil)
	if code := petya.do("POST", "/telegram/link/webapp", api.WebAppInitData{InitData: initData(6006, "Маша")}, nil); code != http.StatusConflict {
		t.Fatalf("taken Telegram: %d", code)
	}

	masha.do("PUT", "/telegram/settings", api.TelegramSettings{NotifyAdded: false, NotifyStock: true}, nil)
	var me api.Me
	masha.do("GET", "/me", nil, &me)
	if me.Telegram == nil || me.Telegram.NotifyAdded || !me.Telegram.NotifyStock {
		t.Fatalf("settings not saved: %+v", me.Telegram)
	}

	if code := masha.do("DELETE", "/telegram/link", nil, nil); code != http.StatusNoContent {
		t.Fatalf("unlink: %d", code)
	}
	var after api.Me // fresh: Decode doesn't clear fields absent from the JSON
	masha.do("GET", "/me", nil, &after)
	if after.Telegram != nil {
		t.Fatal("still linked after unlink")
	}
}

func TestLoginThroughBotOverHTTP(t *testing.T) {
	base, authSvc := newServerWith(t, tgConfig)
	browser := newClient(t, base)

	var start api.TelegramLoginStart
	if code := browser.do("POST", "/auth/telegram/start", nil, &start); code != http.StatusOK {
		t.Fatalf("start: %d", code)
	}
	if !strings.HasPrefix(start.Url, "https://t.me/test_food_bot?start=login_") {
		t.Fatalf("deep link: %s", start.Url)
	}
	poll := api.TelegramLoginCompleteRequest{Token: start.Token}
	if code := browser.do("POST", "/auth/telegram/complete", poll, nil); code != http.StatusAccepted {
		t.Fatalf("before approval: %d", code)
	}

	// The user confirms in the bot (the bot calls this).
	if _, _, err := authSvc.ApproveTelegramLogin(context.Background(), start.Token, auth.TelegramUser{ID: 7007, ChatID: 7007, FirstName: "Оля"}); err != nil {
		t.Fatal(err)
	}

	var me api.Me
	if code := browser.do("POST", "/auth/telegram/complete", poll, &me); code != http.StatusOK || me.User.DisplayName != "Оля" {
		t.Fatalf("after approval: %d %+v", code, me)
	}
	if code := browser.do("GET", "/me", nil, nil); code != http.StatusOK {
		t.Fatalf("no session after Telegram login: %d", code)
	}
	if code := browser.do("POST", "/auth/telegram/complete", poll, nil); code != http.StatusGone {
		t.Fatalf("token reused: %d", code)
	}
}

func TestTelegramDisabledWithoutBot(t *testing.T) {
	c := newClient(t, newServer(t))
	if code := c.do("POST", "/auth/telegram/start", nil, nil); code != http.StatusNotFound {
		t.Fatalf("start without bot: %d", code)
	}
}
