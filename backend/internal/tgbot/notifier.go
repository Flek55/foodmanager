package tgbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
	"foodmanager/internal/telegram"
)

// DefaultDigestDelay: how long to collect events before sending. Adding
// ten items in a row (or an offline queue syncing) becomes one message.
const DefaultDigestDelay = 90 * time.Second

// Notifier implements shopping.Notifier: it collects a household's events
// for a short while, then sends each linked member one digest message
// about what *others* did.
type Notifier struct {
	api    *telegram.Client
	q      *store.Queries
	log    *slog.Logger
	appURL string
	delay  time.Duration

	mu      sync.Mutex
	pending map[uuid.UUID][]event
	timers  map[uuid.UUID]*time.Timer
}

type event struct {
	actor uuid.UUID
	kind  string // "added", "low", "out"
	text  string
}

var _ shopping.Notifier = (*Notifier)(nil)

func NewNotifier(api *telegram.Client, q *store.Queries, log *slog.Logger, appURL string, delay time.Duration) *Notifier {
	return &Notifier{
		api: api, q: q, log: log, appURL: appURL, delay: delay,
		pending: map[uuid.UUID][]event{}, timers: map[uuid.UUID]*time.Timer{},
	}
}

func (n *Notifier) ItemAdded(householdID, actorID uuid.UUID, label string) {
	n.add(householdID, event{actor: actorID, kind: "added", text: label})
}

func (n *Notifier) StockChanged(householdID, actorID uuid.UUID, product, status string) {
	n.add(householdID, event{actor: actorID, kind: status, text: product})
}

func (n *Notifier) add(householdID uuid.UUID, ev event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending[householdID] = append(n.pending[householdID], ev)
	if _, scheduled := n.timers[householdID]; !scheduled {
		n.timers[householdID] = time.AfterFunc(n.delay, func() { n.flush(householdID) })
	}
}

// FlushAll sends everything pending now (on shutdown).
func (n *Notifier) FlushAll() {
	n.mu.Lock()
	ids := make([]uuid.UUID, 0, len(n.pending))
	for id, t := range n.timers {
		t.Stop()
		ids = append(ids, id)
	}
	n.mu.Unlock()
	for _, id := range ids {
		n.flush(id)
	}
}

func (n *Notifier) flush(householdID uuid.UUID) {
	n.mu.Lock()
	events := n.pending[householdID]
	delete(n.pending, householdID)
	delete(n.timers, householdID)
	n.mu.Unlock()
	if len(events) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.send(ctx, householdID, events); err != nil {
		n.log.Error("telegram notify", "household", householdID, "err", err)
	}
}

func (n *Notifier) send(ctx context.Context, householdID uuid.UUID, events []event) error {
	recipients, err := n.q.ListTelegramRecipients(ctx, householdID)
	if err != nil || len(recipients) == 0 {
		return err
	}
	household, err := n.q.GetHouseholdName(ctx, householdID)
	if err != nil {
		return err
	}
	names := map[uuid.UUID]string{}
	nameOf := func(id uuid.UUID) string {
		if name, ok := names[id]; ok {
			return name
		}
		name := "Кто-то"
		if u, err := n.q.GetUserByID(ctx, id); err == nil {
			name = u.DisplayName
		}
		names[id] = name
		return name
	}

	for _, r := range recipients {
		var mine []event
		for _, ev := range events {
			wanted := (ev.kind == "added" && r.NotifyAdded) || (ev.kind != "added" && r.NotifyStock)
			if ev.actor != r.UserID && wanted {
				mine = append(mine, ev)
			}
		}
		if len(mine) == 0 {
			continue
		}
		_, err := n.api.SendMessage(ctx, telegram.SendMessageParams{
			ChatID:      r.ChatID,
			Text:        digestText(household, mine, nameOf),
			ParseMode:   "HTML",
			ReplyMarkup: openAppKeyboard(n.appURL),
		})
		if telegram.IsBlocked(err) {
			// The user blocked the bot: stop trying until they /start again.
			_ = n.q.MarkTelegramBlocked(ctx, r.ChatID)
			continue
		}
		if err != nil {
			n.log.Warn("telegram notify recipient", "user", r.UserID, "err", err)
		}
	}
	return nil
}

// digestText renders e.g.:
//
//	🛒 Ивановы
//	➕ Маша: Молоко 2 л, Хлеб
//	🔴 Петя: закончилось — Кофе
func digestText(household string, events []event, nameOf func(uuid.UUID) string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🛒 <b>%s</b>\n", telegram.Escape(household))

	// Additions grouped per person, in order of first appearance.
	var actors []uuid.UUID
	added := map[uuid.UUID][]string{}
	for _, ev := range events {
		if ev.kind != "added" {
			continue
		}
		if _, seen := added[ev.actor]; !seen {
			actors = append(actors, ev.actor)
		}
		added[ev.actor] = append(added[ev.actor], telegram.Escape(ev.text))
	}
	for _, a := range actors {
		fmt.Fprintf(&b, "➕ %s: %s\n", telegram.Escape(nameOf(a)), strings.Join(added[a], ", "))
	}

	for _, ev := range events {
		switch ev.kind {
		case shopping.StockOut:
			fmt.Fprintf(&b, "🔴 %s: закончилось — %s\n", telegram.Escape(nameOf(ev.actor)), telegram.Escape(ev.text))
		case shopping.StockLow:
			fmt.Fprintf(&b, "🟠 %s: заканчивается — %s\n", telegram.Escape(nameOf(ev.actor)), telegram.Escape(ev.text))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
