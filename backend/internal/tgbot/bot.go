// Package tgbot is the Telegram bot: account linking and login through
// deep links, adding to the list by plain messages, the list with
// "bought" buttons, and digest notifications (see notifier.go).
//
// It talks to Telegram with long polling, so it needs no public URL and
// runs the same on a laptop and on the server.
package tgbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"foodmanager/internal/apperr"
	"foodmanager/internal/auth"
	"foodmanager/internal/household"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
	"foodmanager/internal/telegram"
)

const (
	pollTimeout        = 50 * time.Second
	maxListButtons     = 40
	maxItemsPerMessage = 30
)

type Bot struct {
	api        *telegram.Client
	auth       *auth.Service
	shopping   *shopping.Service
	households *household.Service
	q          *store.Queries
	log        *slog.Logger
	appURL     string // public https URL of the app, "" if unknown
	username   string
}

func New(api *telegram.Client, authSvc *auth.Service, shoppingSvc *shopping.Service, householdSvc *household.Service,
	q *store.Queries, log *slog.Logger, appURL string) *Bot {
	return &Bot{api: api, auth: authSvc, shopping: shoppingSvc, households: householdSvc, q: q, log: log, appURL: appURL}
}

// Setup checks the token, learns the bot's username and registers the
// command menu and, with an https app URL, the Mini App menu button.
func (b *Bot) Setup(ctx context.Context) error {
	me, err := b.api.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("telegram getMe: %w", err)
	}
	b.username = me.Username
	if err := b.api.SetMyCommands(ctx, []telegram.BotCommand{
		{Command: "list", Description: "Список покупок"},
		{Command: "family", Description: "Выбрать семью"},
		{Command: "help", Description: "Что я умею"},
	}); err != nil {
		return fmt.Errorf("telegram setMyCommands: %w", err)
	}
	if isHTTPS(b.appURL) {
		if err := b.api.SetMenuButtonWebApp(ctx, "Покупки", b.appURL); err != nil {
			return fmt.Errorf("telegram setChatMenuButton: %w", err)
		}
	}
	return nil
}

// Username is the bot's @username (without @), known after Setup.
func (b *Bot) Username() string { return b.username }

// Run long-polls Telegram until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		updates, err := b.api.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Warn("telegram getUpdates", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			offset = u.UpdateID + 1
			b.handleSafely(ctx, u)
		}
	}
}

func (b *Bot) handleSafely(ctx context.Context, u telegram.Update) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("telegram handler panic", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch {
	case u.Message != nil:
		b.onMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.onCallback(ctx, u.CallbackQuery)
	}
}

// ---- messages ----

func (b *Bot) onMessage(ctx context.Context, m *telegram.Message) {
	if m.From == nil || m.Chat.Type != "private" {
		return // groups aren't supported: everything here is personal
	}
	tg := tgUser(*m.From, m.Chat.ID)
	text := strings.TrimSpace(m.Text)
	cmd, arg := splitCommand(text)
	switch cmd {
	case "start":
		b.onStart(ctx, tg, arg)
	case "list":
		if user, acc, ok := b.linkedUser(ctx, tg); ok {
			if hid, ok := b.household(ctx, tg, user, acc); ok {
				b.sendList(ctx, tg.ChatID, user, hid)
			}
		}
	case "family":
		if user, _, ok := b.linkedUser(ctx, tg); ok {
			b.chooseHousehold(ctx, tg.ChatID, user)
		}
	case "help":
		b.say(ctx, tg.ChatID, helpText, nil)
	case "":
		if text != "" {
			b.addFromText(ctx, tg, text)
		}
	default:
		b.say(ctx, tg.ChatID, "Не знаю такой команды.\n\n"+helpText, nil)
	}
}

const helpText = `Что я умею:
• Пришлите покупки сообщением — добавлю в список. Можно сразу несколько и с количеством: <i>молоко 2л, хлеб, яйца 10</i>
• /list — список покупок; нажмите на товар, когда купите
• /family — выбрать семью, если их несколько
• Уведомления: что добавили в список и что закончилось дома. Настраиваются в приложении → Профиль`

func (b *Bot) onStart(ctx context.Context, tg auth.TelegramUser, arg string) {
	switch {
	case strings.HasPrefix(arg, auth.LinkTokenPrefix):
		user, err := b.auth.LinkTelegramByToken(ctx, arg, tg)
		if err != nil {
			b.sayError(ctx, tg.ChatID, err)
			return
		}
		b.say(ctx, tg.ChatID, fmt.Sprintf(
			"Готово, %s! Telegram привязан.\n\nТеперь я буду присылать, что добавили в список и что закончилось дома. А покупки можно писать мне прямо сюда: <i>молоко 2л, хлеб</i>.",
			telegram.Escape(user.DisplayName)), b.appKeyboard())

	case strings.HasPrefix(arg, auth.LoginTokenPrefix):
		ua, err := b.auth.TelegramLoginRequest(ctx, arg)
		if err != nil {
			b.say(ctx, tg.ChatID, "Ссылка для входа устарела. Нажмите «Войти через Telegram» ещё раз.", nil)
			return
		}
		b.say(ctx, tg.ChatID, fmt.Sprintf(
			"Войти в приложение «Покупки»?\n\nЗапрос с устройства: <b>%s</b>\n\nЕсли вход начинали не вы — нажмите «Нет»: кто-то пытается войти в ваш аккаунт.",
			telegram.Escape(describeDevice(ua))),
			&telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "✅ Да, это я", CallbackData: "la:" + arg},
				{Text: "✖ Нет", CallbackData: "lr:" + arg},
			}}})

	default:
		if user, _, err := b.auth.UserByTelegram(ctx, tg.ID); err == nil {
			b.unblock(ctx, tg)
			b.say(ctx, tg.ChatID, fmt.Sprintf("С возвращением, %s!\n\n%s", telegram.Escape(user.DisplayName), helpText), b.appKeyboard())
			return
		}
		b.say(ctx, tg.ChatID, notLinkedText, b.appKeyboard())
	}
}

const notLinkedText = `Привет! Я бот семейного списка покупок.

Чтобы я мог добавлять покупки и присылать уведомления, привяжите аккаунт: в приложении откройте <b>Профиль → Подключить Telegram</b>.

Нет аккаунта? На странице входа нажмите <b>«Войти через Telegram»</b>.`

func (b *Bot) addFromText(ctx context.Context, tg auth.TelegramUser, text string) {
	user, acc, ok := b.linkedUser(ctx, tg)
	if !ok {
		return
	}
	hid, ok := b.household(ctx, tg, user, acc)
	if !ok {
		return
	}
	parts := shopping.SplitItems(text)
	if len(parts) > maxItemsPerMessage {
		parts = parts[:maxItemsPerMessage]
	}

	var added, existing, failed []string
	for _, part := range parts {
		p := shopping.ParseItem(part)
		item, created, err := b.shopping.AddItem(ctx, user.ID, hid, shopping.AddItemInput{
			Name: p.Name, Quantity: p.Quantity, Unit: p.Unit,
		})
		label := telegram.Escape(itemLabel(item, p))
		switch {
		case err != nil:
			failed = append(failed, fmt.Sprintf("%s — %s", telegram.Escape(p.Name), telegram.Escape(errMessage(err))))
		case created:
			added = append(added, label)
		default:
			existing = append(existing, label)
		}
	}

	name, _ := b.q.GetHouseholdName(ctx, hid)
	var out strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&out, "Добавлено в «%s»:\n• %s\n", telegram.Escape(name), strings.Join(added, "\n• "))
	}
	if len(existing) > 0 {
		fmt.Fprintf(&out, "Уже было в списке (обновлено):\n• %s\n", strings.Join(existing, "\n• "))
	}
	if len(failed) > 0 {
		fmt.Fprintf(&out, "Не получилось:\n• %s\n", strings.Join(failed, "\n• "))
	}
	b.say(ctx, tg.ChatID, strings.TrimSpace(out.String()), &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "🛒 Показать список", CallbackData: "r:" + hid.String()}}},
	})
}

// itemLabel is "Молоко 2 л" for an added item, falling back to the parsed
// input when the add failed.
func itemLabel(item store.ListItemViewsRow, p shopping.ParsedItem) string {
	if item.Name != "" {
		return shopping.ItemLabel(item.Name, item.Quantity, item.Unit)
	}
	return shopping.ItemLabel(p.Name, p.Quantity, p.Unit)
}

// ---- the list ----

func (b *Bot) sendList(ctx context.Context, chatID int64, user store.User, hid uuid.UUID) {
	text, kb, err := b.renderList(ctx, user, hid)
	if err != nil {
		b.sayError(ctx, chatID, err)
		return
	}
	b.say(ctx, chatID, text, kb)
}

func (b *Bot) renderList(ctx context.Context, user store.User, hid uuid.UUID) (string, *telegram.InlineKeyboardMarkup, error) {
	snap, err := b.shopping.Snapshot(ctx, user.ID, hid)
	if err != nil {
		return "", nil, err
	}
	name, _ := b.q.GetHouseholdName(ctx, hid)

	order := map[uuid.UUID]int{}
	for i, c := range snap.Categories {
		order[c.ID] = i
	}
	rank := func(it store.ListItemViewsRow) int {
		if it.CategoryID != nil {
			if i, ok := order[*it.CategoryID]; ok {
				return i
			}
		}
		return len(order)
	}
	var open []store.ListItemViewsRow
	for _, it := range snap.Items {
		if it.CheckedAt == nil {
			open = append(open, it)
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		if ri, rj := rank(open[i]), rank(open[j]); ri != rj {
			return ri < rj
		}
		return open[i].Name < open[j].Name
	})

	var rows [][]telegram.InlineKeyboardButton
	for i, it := range open {
		if i == maxListButtons {
			break
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         truncate("◻️ "+shopping.ItemLabel(it.Name, it.Quantity, it.Unit), 60),
			CallbackData: "c:" + it.ID.String(),
		}})
	}
	footer := []telegram.InlineKeyboardButton{{Text: "🔄 Обновить", CallbackData: "r:" + hid.String()}}
	if isHTTPS(b.appURL) {
		footer = append(footer, telegram.InlineKeyboardButton{Text: "📱 Приложение", WebApp: &telegram.WebAppInfo{URL: b.appURL}})
	}
	rows = append(rows, footer)

	var text string
	switch {
	case len(open) == 0:
		text = fmt.Sprintf("🛒 <b>%s</b>\n\nСписок пуст 🎉", telegram.Escape(name))
	case len(open) > maxListButtons:
		text = fmt.Sprintf("🛒 <b>%s</b> — %d позиций\nНажмите на товар, когда купите. Показаны первые %d — остальные в приложении.",
			telegram.Escape(name), len(open), maxListButtons)
	default:
		text = fmt.Sprintf("🛒 <b>%s</b> — %s\nНажмите на товар, когда купите.", telegram.Escape(name), positions(len(open)))
	}
	return text, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

// ---- households ----

// household picks the household to work with: the chosen default, the
// only one, or — with several and no choice yet — asks.
func (b *Bot) household(ctx context.Context, tg auth.TelegramUser, user store.User, acc store.TelegramAccount) (uuid.UUID, bool) {
	hs, err := b.households.ListForUser(ctx, user.ID)
	if err != nil {
		b.sayError(ctx, tg.ChatID, err)
		return uuid.Nil, false
	}
	if len(hs) == 0 {
		b.say(ctx, tg.ChatID, "Вы пока не состоите ни в одной семье. Создайте семью или вступите по коду в приложении.", b.appKeyboard())
		return uuid.Nil, false
	}
	if acc.DefaultHouseholdID != nil {
		for _, h := range hs {
			if h.ID == *acc.DefaultHouseholdID {
				return h.ID, true
			}
		}
	}
	if len(hs) == 1 {
		return hs[0].ID, true
	}
	b.chooseHousehold(ctx, tg.ChatID, user)
	return uuid.Nil, false
}

func (b *Bot) chooseHousehold(ctx context.Context, chatID int64, user store.User) {
	hs, err := b.households.ListForUser(ctx, user.ID)
	if err != nil {
		b.sayError(ctx, chatID, err)
		return
	}
	if len(hs) == 0 {
		b.say(ctx, chatID, "Вы пока не состоите ни в одной семье.", b.appKeyboard())
		return
	}
	buttons := make([]telegram.InlineKeyboardButton, len(hs))
	for i, h := range hs {
		buttons[i] = telegram.InlineKeyboardButton{Text: h.Name, CallbackData: "h:" + h.ID.String()}
	}
	b.say(ctx, chatID, "В какую семью добавлять покупки?", telegram.Keyboard(buttons...))
}

// ---- inline buttons ----

func (b *Bot) onCallback(ctx context.Context, cq *telegram.CallbackQuery) {
	if cq.Message == nil {
		return
	}
	chatID, msgID := cq.Message.Chat.ID, cq.Message.MessageID
	tg := tgUser(cq.From, chatID)
	kind, data, _ := strings.Cut(cq.Data, ":")
	toast := ""
	defer func() { _ = b.api.AnswerCallbackQuery(ctx, cq.ID, toast) }()

	switch kind {
	case "la": // approve login
		user, created, err := b.auth.ApproveTelegramLogin(ctx, data, tg)
		if err != nil {
			b.edit(ctx, chatID, msgID, "Ссылка для входа устарела. Нажмите «Войти через Telegram» ещё раз.", nil)
			return
		}
		text := fmt.Sprintf("✅ Вы вошли как <b>%s</b>. Вернитесь в приложение.", telegram.Escape(user.DisplayName))
		if created {
			text += "\n\nСоздан новый аккаунт. Если у вас уже был аккаунт с паролем — войдите по паролю и привяжите Telegram в профиле."
		}
		b.edit(ctx, chatID, msgID, text, nil)

	case "lr": // reject login
		_ = b.auth.RejectTelegramLogin(ctx, data, tg)
		b.edit(ctx, chatID, msgID, "Вход отменён. Если это были не вы — ничего страшного: без вашего подтверждения войти нельзя.", nil)

	case "c": // bought
		user, _, ok := b.linkedUser(ctx, tg)
		if !ok {
			return
		}
		itemID, err := uuid.Parse(data)
		if err != nil {
			return
		}
		row, err := b.q.GetItemHousehold(ctx, itemID)
		if errors.Is(err, pgx.ErrNoRows) {
			toast = "Этой позиции уже нет"
			return
		}
		if err != nil {
			b.log.Error("telegram check item", "err", err)
			return
		}
		item, err := b.shopping.SetChecked(ctx, user.ID, row.HouseholdID, itemID, true)
		if err != nil {
			toast = errMessage(err)
		} else {
			toast = "✓ Куплено: " + item.Name
		}
		b.refreshList(ctx, chatID, msgID, user, row.HouseholdID)

	case "r": // refresh / show list
		user, _, ok := b.linkedUser(ctx, tg)
		if !ok {
			return
		}
		hid, err := uuid.Parse(data)
		if err != nil {
			return
		}
		if cq.Message.Text != "" && strings.HasPrefix(cq.Message.Text, "🛒") {
			b.refreshList(ctx, chatID, msgID, user, hid)
		} else {
			b.sendList(ctx, chatID, user, hid) // the button was under an "added" reply
		}

	case "h": // choose household
		user, _, ok := b.linkedUser(ctx, tg)
		if !ok {
			return
		}
		hid, err := uuid.Parse(data)
		if err != nil {
			return
		}
		if _, err := b.households.Get(ctx, user.ID, hid); err != nil {
			toast = errMessage(err)
			return
		}
		if err := b.q.SetTelegramDefaultHousehold(ctx, store.SetTelegramDefaultHouseholdParams{HouseholdID: &hid, UserID: user.ID}); err != nil {
			b.log.Error("telegram set household", "err", err)
			return
		}
		name, _ := b.q.GetHouseholdName(ctx, hid)
		b.edit(ctx, chatID, msgID, fmt.Sprintf("Теперь добавляю покупки в «%s». Сменить — /family", telegram.Escape(name)), nil)
	}
}

func (b *Bot) refreshList(ctx context.Context, chatID, msgID int64, user store.User, hid uuid.UUID) {
	text, kb, err := b.renderList(ctx, user, hid)
	if err != nil {
		b.sayError(ctx, chatID, err)
		return
	}
	b.edit(ctx, chatID, msgID, text, kb)
}

// ---- helpers ----

// linkedUser resolves the Telegram user to their account, or explains how
// to link one.
func (b *Bot) linkedUser(ctx context.Context, tg auth.TelegramUser) (store.User, store.TelegramAccount, bool) {
	user, acc, err := b.auth.UserByTelegram(ctx, tg.ID)
	if err != nil {
		if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindNotFound {
			b.log.Error("telegram user lookup", "err", err)
		}
		b.say(ctx, tg.ChatID, notLinkedText, b.appKeyboard())
		return user, acc, false
	}
	if acc.BlockedAt != nil {
		b.unblock(ctx, tg)
	}
	return user, acc, true
}

func (b *Bot) unblock(ctx context.Context, tg auth.TelegramUser) {
	if err := b.q.UnblockTelegram(ctx, tg.ID); err != nil {
		b.log.Warn("telegram unblock", "err", err)
	}
}

func (b *Bot) say(ctx context.Context, chatID int64, text string, kb *telegram.InlineKeyboardMarkup) {
	if _, err := b.api.SendMessage(ctx, telegram.SendMessageParams{ChatID: chatID, Text: text, ParseMode: "HTML", ReplyMarkup: kb}); err != nil {
		b.log.Warn("telegram send", "chat", chatID, "err", err)
	}
}

func (b *Bot) edit(ctx context.Context, chatID, msgID int64, text string, kb *telegram.InlineKeyboardMarkup) {
	if err := b.api.EditMessageText(ctx, telegram.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ParseMode: "HTML", ReplyMarkup: kb,
	}); err != nil {
		b.log.Warn("telegram edit", "chat", chatID, "err", err)
	}
}

func (b *Bot) sayError(ctx context.Context, chatID int64, err error) {
	if _, ok := apperr.As(err); !ok {
		b.log.Error("telegram handler", "err", err)
	}
	b.say(ctx, chatID, telegram.Escape(errMessage(err)), nil)
}

// appKeyboard is an "open the app" button, if the app has an https URL
// (Telegram rejects other URLs in buttons).
func (b *Bot) appKeyboard() *telegram.InlineKeyboardMarkup {
	return openAppKeyboard(b.appURL)
}

func openAppKeyboard(appURL string) *telegram.InlineKeyboardMarkup {
	if !isHTTPS(appURL) {
		return nil
	}
	return telegram.Keyboard(telegram.InlineKeyboardButton{Text: "📱 Открыть приложение", WebApp: &telegram.WebAppInfo{URL: appURL}})
}

func isHTTPS(u string) bool { return strings.HasPrefix(u, "https://") }

// errMessage turns a domain error into a sentence for the chat.
func errMessage(err error) string {
	if e, ok := apperr.As(err); ok {
		return capitalize(e.Message)
	}
	return "Что-то пошло не так, попробуйте ещё раз."
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func tgUser(u telegram.User, chatID int64) auth.TelegramUser {
	return auth.TelegramUser{ID: u.ID, ChatID: chatID, Username: u.Username, FirstName: u.FirstName, LastName: u.LastName}
}

// splitCommand: "/start link_x" → ("start", "link_x"); "/list@my_bot" →
// ("list", ""); plain text → ("", "").
func splitCommand(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	cmd, arg, _ := strings.Cut(text[1:], " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(arg)
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func positions(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return fmt.Sprintf("%d позиция", n)
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d позиции", n)
	}
	return fmt.Sprintf("%d позиций", n)
}

// describeDevice turns a User-Agent into "Chrome на Windows" for the login
// confirmation, so the user can recognise their own device.
func describeDevice(ua string) string {
	if ua == "" {
		return "неизвестное устройство"
	}
	os := "неизвестной системе"
	for _, c := range []struct{ marker, name string }{
		{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"},
		{"Windows", "Windows"}, {"Mac OS", "Mac"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, c.marker) {
			os = c.name
			break
		}
	}
	browser := "браузер"
	for _, c := range []struct{ marker, name string }{
		{"Edg/", "Edge"}, {"YaBrowser", "Яндекс Браузер"}, {"OPR/", "Opera"},
		{"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"},
	} {
		if strings.Contains(ua, c.marker) {
			browser = c.name
			break
		}
	}
	return browser + " на " + os
}
