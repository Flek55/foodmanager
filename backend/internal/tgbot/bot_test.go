package tgbot

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/auth"
	"foodmanager/internal/events"
	"foodmanager/internal/household"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
	"foodmanager/internal/telegram"
	"foodmanager/internal/telegram/telegramtest"
	"foodmanager/internal/testdb"
)

type env struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	fake       *telegramtest.Server
	auth       *auth.Service
	shopping   *shopping.Service
	households *household.Service
	q          *store.Queries
}

func setup(t *testing.T) env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool := testdb.New(t)
	fake := telegramtest.New(t)
	api := telegram.New(telegramtest.Token, fake.URL)
	log := slog.New(slog.DiscardHandler)

	e := env{
		ctx: ctx, pool: pool, fake: fake,
		auth:       auth.NewService(pool),
		shopping:   shopping.NewService(pool, events.Nop{}),
		households: household.NewService(pool, events.Nop{}),
		q:          store.New(pool),
	}
	// A short digest delay keeps the tests fast.
	e.shopping.SetNotifier(NewNotifier(api, e.q, log, "https://app.example", 100*time.Millisecond))

	bot := New(api, e.auth, e.shopping, e.households, e.q, log, "https://app.example")
	if err := bot.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	go bot.Run(ctx)
	return e
}

// person registers a user; linked ones also go through the bot's /start.
func (e env) person(t *testing.T, name string, tgID int64, link bool) (store.User, telegram.User) {
	t.Helper()
	user, _, err := e.auth.Register(e.ctx, strings.ToLower(uuid.NewString()[:8])+"@example.com", "password123", name, "")
	if err != nil {
		t.Fatal(err)
	}
	tg := telegram.User{ID: tgID, FirstName: name}
	if link {
		token, _, err := e.auth.NewTelegramLinkToken(e.ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		e.fake.SendText(tg, "/start "+token)
		e.fake.WaitSent(t, tgID, "Telegram привязан")
	}
	return user, tg
}

func (e env) family(t *testing.T, owner store.User, name string, members ...store.User) uuid.UUID {
	t.Helper()
	h, err := e.households.Create(e.ctx, owner.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := e.households.CreateInvite(e.ctx, owner.ID, h.ID, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if _, err := e.households.AcceptInvite(e.ctx, m.ID, inv.Code); err != nil {
			t.Fatal(err)
		}
	}
	return h.ID
}

func TestLinkAndAddByMessage(t *testing.T) {
	e := setup(t)
	masha, tg := e.person(t, "Маша", 1001, true)
	hid := e.family(t, masha, "Ивановы")

	e.fake.SendText(tg, "молоко 2л, хлеб, Молоко 3,2%")
	msg := e.fake.WaitSent(t, tg.ID, "Добавлено в «Ивановы»")
	for _, want := range []string{"Молоко 2 л", "Хлеб", "Молоко 3,2%"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("reply %q misses %q", msg.Text, want)
		}
	}
	snap, _ := e.shopping.Snapshot(e.ctx, masha.ID, hid)
	if len(snap.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(snap.Items))
	}

	// The same item again is reported as already on the list.
	e.fake.SendText(tg, "хлеб")
	e.fake.WaitSent(t, tg.ID, "Уже было в списке")

	// A stranger gets instructions, not access.
	stranger := telegram.User{ID: 999, FirstName: "Чужой"}
	e.fake.SendText(stranger, "хлеб")
	e.fake.WaitSent(t, stranger.ID, "привяжите аккаунт")
}

func TestLinkTokenIsSingleUse(t *testing.T) {
	e := setup(t)
	masha, _ := e.person(t, "Маша", 1001, false)
	token, _, _ := e.auth.NewTelegramLinkToken(e.ctx, masha.ID)

	e.fake.SendText(telegram.User{ID: 1001, FirstName: "Маша"}, "/start "+token)
	e.fake.WaitSent(t, 1001, "Telegram привязан")

	// Someone who got hold of the same link can't attach their Telegram.
	e.fake.SendText(telegram.User{ID: 2002, FirstName: "Злоумышленник"}, "/start "+token)
	e.fake.WaitSent(t, 2002, "устарела")
	if _, _, err := e.auth.UserByTelegram(e.ctx, 2002); err == nil {
		t.Fatal("reused token linked a second Telegram")
	}
}

func TestListAndBoughtButton(t *testing.T) {
	e := setup(t)
	masha, tg := e.person(t, "Маша", 1001, true)
	hid := e.family(t, masha, "Ивановы")
	bread, _, _ := e.shopping.AddItem(e.ctx, masha.ID, hid, shopping.AddItemInput{Name: "Хлеб"})

	e.fake.SendText(tg, "/list")
	list := e.fake.WaitSent(t, tg.ID, "🛒 <b>Ивановы</b> — 1 позиция")
	if list.ReplyMarkup == nil || list.ReplyMarkup.InlineKeyboard[0][0].CallbackData != "c:"+bread.ID.String() {
		t.Fatalf("no bought button for bread: %+v", list.ReplyMarkup)
	}

	e.fake.Press(tg, list, "c:"+bread.ID.String())
	e.fake.WaitEdited(t, "Список пуст")
	snap, _ := e.shopping.Snapshot(e.ctx, masha.ID, hid)
	if snap.Items[0].CheckedAt == nil {
		t.Fatal("bread not checked off")
	}
}

func TestDigestNotifications(t *testing.T) {
	e := setup(t)
	masha, tgMasha := e.person(t, "Маша", 1001, true)
	petya, tgPetya := e.person(t, "Петя", 1002, true)
	hid := e.family(t, masha, "Ивановы", petya)

	// Petya adds two items and runs out of coffee: Masha gets ONE message.
	e.shopping.AddItem(e.ctx, petya.ID, hid, shopping.AddItemInput{Name: "Хлеб"})
	e.shopping.AddItem(e.ctx, petya.ID, hid, shopping.AddItemInput{Name: "Сыр"})
	e.shopping.TrackProduct(e.ctx, petya.ID, hid, "Кофе", shopping.StockOut, nil, nil)

	msg := e.fake.WaitSent(t, tgMasha.ID, "Ивановы")
	for _, want := range []string{"➕ Петя: Хлеб, Сыр", "🔴 Петя: закончилось — Кофе"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("digest %q misses %q", msg.Text, want)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if n := len(e.fake.Sent(tgMasha.ID)); n != 2 { // "привязан" + one digest
		t.Errorf("Masha got %d messages, want 2", n)
	}
	if n := len(e.fake.Sent(tgPetya.ID)); n != 1 { // only "привязан": no echo of own actions
		t.Errorf("Petya got %d messages, want 1 (no notifications about himself)", n)
	}

	// Masha turns off "added" notifications: only stock news reaches her.
	if _, err := e.auth.UpdateTelegramSettings(e.ctx, masha.ID, false, true); err != nil {
		t.Fatal(err)
	}
	e.shopping.AddItem(e.ctx, petya.ID, hid, shopping.AddItemInput{Name: "Масло"})
	e.shopping.TrackProduct(e.ctx, petya.ID, hid, "Сахар", shopping.StockLow, nil, nil)
	msg = e.fake.WaitSent(t, tgMasha.ID, "Сахар")
	if strings.Contains(msg.Text, "Масло") {
		t.Errorf("muted 'added' notification delivered: %q", msg.Text)
	}

	// Masha blocks the bot: the account is marked and skipped from then on.
	e.fake.Block(tgMasha.ID)
	e.shopping.TrackProduct(e.ctx, petya.ID, hid, "Чай", shopping.StockOut, nil, nil)
	deadline := time.Now().Add(3 * time.Second)
	for {
		acc, _ := e.q.GetTelegramAccount(e.ctx, masha.ID)
		if acc.BlockedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("blocked recipient not marked")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLoginThroughBot(t *testing.T) {
	e := setup(t)
	masha, tg := e.person(t, "Маша", 1001, true)
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/140.0 Safari/537.36"

	token, _, err := e.auth.StartTelegramLogin(e.ctx, ua)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, status, _ := e.auth.CompleteTelegramLogin(e.ctx, token, ua); status != auth.LoginPending {
		t.Fatalf("before confirmation: %s", status)
	}

	e.fake.SendText(tg, "/start "+token)
	prompt := e.fake.WaitSent(t, tg.ID, "Войти в приложение")
	if !strings.Contains(prompt.Text, "Chrome на Windows") {
		t.Errorf("device not shown: %q", prompt.Text)
	}
	e.fake.Press(tg, prompt, "la:"+token)
	e.fake.WaitEdited(t, "Вы вошли как <b>Маша</b>")

	user, sess, status, err := e.auth.CompleteTelegramLogin(e.ctx, token, ua)
	if err != nil || status != auth.LoginApproved || user.ID != masha.ID || sess.Token == "" {
		t.Fatalf("complete: %v %v %v", status, user.ID, err)
	}
	// A token logs in exactly once.
	if _, _, status, _ := e.auth.CompleteTelegramLogin(e.ctx, token, ua); status != auth.LoginExpired {
		t.Fatalf("second complete: %s", status)
	}
}

func TestLoginRejectedAndNewAccount(t *testing.T) {
	e := setup(t)

	// "No" in the bot: the waiting page gets "expired", no session.
	token, _, _ := e.auth.StartTelegramLogin(e.ctx, "")
	vasya := telegram.User{ID: 3003, FirstName: "Вася"}
	e.fake.SendText(vasya, "/start "+token)
	prompt := e.fake.WaitSent(t, vasya.ID, "Войти в приложение")
	e.fake.Press(vasya, prompt, "lr:"+token)
	e.fake.WaitEdited(t, "Вход отменён")
	if _, _, status, _ := e.auth.CompleteTelegramLogin(e.ctx, token, ""); status != auth.LoginExpired {
		t.Fatalf("rejected login: %s", status)
	}

	// An unknown Telegram that confirms gets a fresh account.
	token, _, _ = e.auth.StartTelegramLogin(e.ctx, "")
	e.fake.SendText(vasya, "/start "+token)
	prompt = e.fake.WaitSent(t, vasya.ID, "Войти в приложение")
	e.fake.Press(vasya, prompt, "la:"+token)
	e.fake.WaitEdited(t, "Создан новый аккаунт")
	user, _, status, _ := e.auth.CompleteTelegramLogin(e.ctx, token, "")
	if status != auth.LoginApproved || user.DisplayName != "Вася" || user.Email != nil {
		t.Fatalf("new account: %v %+v", status, user)
	}
}

func TestSeveralHouseholdsAskWhichOne(t *testing.T) {
	e := setup(t)
	masha, tg := e.person(t, "Маша", 1001, true)
	e.family(t, masha, "Ивановы")
	dacha := e.family(t, masha, "Дача")

	e.fake.SendText(tg, "хлеб")
	ask := e.fake.WaitSent(t, tg.ID, "В какую семью")
	e.fake.Press(tg, ask, "h:"+dacha.String())
	e.fake.WaitEdited(t, "Теперь добавляю покупки в «Дача»")

	e.fake.SendText(tg, "уголь")
	e.fake.WaitSent(t, tg.ID, "Добавлено в «Дача»")
}
