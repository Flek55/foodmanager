package shopping

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Notifier hears about changes worth telling other family members about
// (the Telegram bot implements it). Calls happen after the change is
// committed and must not block.
type Notifier interface {
	// ItemAdded: a product was newly put on the list; label is e.g. "Молоко 2 л".
	ItemAdded(householdID, actorID uuid.UUID, label string)
	// StockChanged: a tracked product became "low" or "out".
	StockChanged(householdID, actorID uuid.UUID, product, status string)
}

type nopNotifier struct{}

func (nopNotifier) ItemAdded(uuid.UUID, uuid.UUID, string)            {}
func (nopNotifier) StockChanged(uuid.UUID, uuid.UUID, string, string) {}

// SetNotifier plugs in a notifier (the default one does nothing).
func (s *Service) SetNotifier(n Notifier) { s.notify = n }

var unitLabels = map[string]string{"pcs": "шт", "kg": "кг", "g": "г", "l": "л", "ml": "мл", "pack": "уп"}

// FormatQuantity renders "2 л", "1,5 кг"; "" without a quantity.
func FormatQuantity(qty *float64, unit *string) string {
	if qty == nil {
		return ""
	}
	label := "шт"
	if unit != nil {
		label = unitLabels[*unit]
	}
	return strings.Replace(strconv.FormatFloat(*qty, 'f', -1, 64), ".", ",", 1) + " " + label
}

// ItemLabel renders an item as people say it: "Молоко 2 л".
func ItemLabel(name string, qty *float64, unit *string) string {
	if q := FormatQuantity(qty, unit); q != "" {
		return name + " " + q
	}
	return name
}
