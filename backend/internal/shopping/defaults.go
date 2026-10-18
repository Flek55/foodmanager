package shopping

import (
	"context"

	"github.com/google/uuid"

	"foodmanager/internal/store"
)

// DefaultCategory is a built-in category every household starts with.
// Keep in sync with the seed in migrations/00002_shopping_list.sql.
type DefaultCategory struct {
	Key   string
	Name  string
	Emoji string
}

// Ordered roughly the way aisles go in a typical Russian supermarket.
var DefaultCategories = []DefaultCategory{
	{"produce", "Овощи и фрукты", "🥦"},
	{"bakery", "Хлеб и выпечка", "🥖"},
	{"dairy", "Молочное и яйца", "🥛"},
	{"meat", "Мясо и рыба", "🥩"},
	{"grocery", "Бакалея", "🍝"},
	{"frozen", "Заморозка", "🧊"},
	{"drinks", "Напитки", "🧃"},
	{"sweets", "Сладкое и снеки", "🍫"},
	{"household", "Бытовая химия", "🧽"},
	{"hygiene", "Гигиена и красота", "🧴"},
	{"pharmacy", "Аптека", "💊"},
	{"kids", "Детское", "🧸"},
	{"pets", "Для животных", "🐾"},
	{"other", "Другое", "📦"},
}

// SeedCategories creates the built-in categories for a new household.
// It runs inside the caller's transaction.
func SeedCategories(ctx context.Context, q *store.Queries, householdID uuid.UUID, version int64) error {
	for i, c := range DefaultCategories {
		_, err := q.CreateCategory(ctx, store.CreateCategoryParams{
			HouseholdID: householdID,
			Key:         &c.Key,
			Name:        c.Name,
			Emoji:       &c.Emoji,
			SortOrder:   int32(i + 1),
			Version:     version,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
