package shopping

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
	"foodmanager/internal/store"
	"foodmanager/internal/testdb"
)

func TestGuessCategory(t *testing.T) {
	cases := map[string]string{
		"Молоко 3,2%":              "dairy",
		"сливочное масло":          "dairy",
		"Масло подсолнечное":       "grocery",
		"Зубная паста":             "hygiene",
		"паста карбонара":          "grocery",
		"Таблетки для посудомойки": "household",
		"Корм для кошек":           "pets",
		"Детское пюре":             "kids",
		"Пельмени":                 "frozen",
		"Шоколадка":                "sweets",
		"Помидоры черри":           "produce",
		"Зелёный горошек":          "grocery",
		"Куриная печень":           "meat",
		"Печенье":                  "sweets",
		"Туалетная бумага":         "household",
		"Фетучини":                 "grocery",
		"Хлеб бородинский":         "bakery",
		"Вода минеральная":         "drinks",
		"Нурофен":                  "pharmacy",
		"Что-то совсем непонятное": "other",
		"леденцы":                  "sweets",
		"ЯЙЦА":                     "dairy",
	}
	for name, want := range cases {
		if got := guessCategoryKey(name); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

// recorder is an events.Publisher that remembers what was published.
type recorder struct {
	mu     sync.Mutex
	events []events.Event
}

func (r *recorder) Publish(_ uuid.UUID, ev events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

type fixture struct {
	ctx       context.Context
	pool      *pgxpool.Pool
	svc       *Service
	pub       *recorder
	user      uuid.UUID
	household uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)
	u, err := q.CreateUser(ctx, store.CreateUserParams{DisplayName: "Маша"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := q.CreateHousehold(ctx, store.CreateHouseholdParams{Name: "Ивановы", CreatedBy: &u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AddMember(ctx, store.AddMemberParams{HouseholdID: h.ID, UserID: u.ID, Role: "owner"}); err != nil {
		t.Fatal(err)
	}
	if err := SeedCategories(ctx, q, h.ID, 1); err != nil {
		t.Fatal(err)
	}
	pub := &recorder{}
	return fixture{ctx: ctx, pool: pool, svc: NewService(pool, pub), pub: pub, user: u.ID, household: h.ID}
}

func (f fixture) add(t *testing.T, in AddItemInput) store.ListItemViewsRow {
	t.Helper()
	item, _, err := f.svc.AddItem(f.ctx, f.user, f.household, in)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func (f fixture) categoryKey(t *testing.T, id *uuid.UUID) string {
	t.Helper()
	if id == nil {
		return ""
	}
	c, err := store.New(f.pool).GetCategory(f.ctx, store.GetCategoryParams{HouseholdID: f.household, ID: *id})
	if err != nil {
		t.Fatal(err)
	}
	return *c.Key
}

func TestAddItemCreatesProductAndGuessesCategory(t *testing.T) {
	f := setup(t)
	qty := 2.0
	item := f.add(t, AddItemInput{Name: "  Молоко   3,2% ", Quantity: &qty})

	if item.Name != "Молоко 3,2%" || *item.Unit != "pcs" || *item.Quantity != 2 {
		t.Fatalf("unexpected item %+v", item)
	}
	if got := f.categoryKey(t, item.CategoryID); got != "dairy" {
		t.Fatalf("category: %q", got)
	}
	if len(f.pub.events) != 1 || f.pub.events[0].Kind != events.KindList {
		t.Fatalf("expected one list event, got %+v", f.pub.events)
	}

	eggs := f.add(t, AddItemInput{Name: "яйца"})
	if eggs.Name != "Яйца" {
		t.Fatalf("new product name should be capitalized, got %q", eggs.Name)
	}
}

func TestReAddMergesIntoActiveItem(t *testing.T) {
	f := setup(t)
	first := f.add(t, AddItemInput{Name: "Хлеб"})

	qty, unit, note := 2.0, "pcs", "бородинский"
	again, created, err := f.svc.AddItem(f.ctx, f.user, f.household, AddItemInput{Name: "хлеб", Quantity: &qty, Unit: &unit, Note: &note})
	if err != nil {
		t.Fatal(err)
	}
	if created || again.ID != first.ID || *again.Quantity != 2 || *again.Note != "бородинский" {
		t.Fatalf("merge: created=%v %+v", created, again)
	}

	// Once bought, adding again creates a new item.
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, first.ID, true); err != nil {
		t.Fatal(err)
	}
	third, created, err := f.svc.AddItem(f.ctx, f.user, f.household, AddItemInput{Name: "Хлеб"})
	if err != nil || !created || third.ID == first.ID {
		t.Fatalf("after check: created=%v id=%v err=%v", created, third.ID, err)
	}

	// Unchecking the old one would duplicate the product on the list.
	_, err = f.svc.SetChecked(f.ctx, f.user, f.household, first.ID, false)
	assertKind(t, err, apperr.KindConflict)
}

func TestAddItemIsIdempotentByClientID(t *testing.T) {
	f := setup(t)
	id := uuid.Must(uuid.NewV7())
	a := f.add(t, AddItemInput{ID: &id, Name: "Сыр"})
	b := f.add(t, AddItemInput{ID: &id, Name: "Сыр"})
	if a.ID != id || b.ID != id {
		t.Fatalf("ids: %v %v", a.ID, b.ID)
	}
	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	if len(snap.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(snap.Items))
	}
}

func TestUpdateItem(t *testing.T) {
	f := setup(t)
	item := f.add(t, AddItemInput{Name: "Молоко"})
	other := f.add(t, AddItemInput{Name: "Кефир"})
	st, err := f.svc.CreateStore(f.ctx, f.user, f.household, "Ашан")
	if err != nil {
		t.Fatal(err)
	}

	qty, unit, desc := 1.5, "l", "Простоквашино"
	updated, err := f.svc.UpdateItem(f.ctx, f.user, f.household, item.ID, UpdateItemInput{
		Name: "Молоко 2,5%", Description: &desc, CategoryID: item.CategoryID,
		Quantity: &qty, Unit: &unit, StoreID: &st.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Молоко 2,5%" || *updated.Description != desc || *updated.StoreID != st.ID || *updated.Unit != "l" {
		t.Fatalf("unexpected %+v", updated)
	}

	// PUT semantics: omitted optional fields are cleared.
	cleared, err := f.svc.UpdateItem(f.ctx, f.user, f.household, item.ID, UpdateItemInput{Name: "Молоко 2,5%"})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Quantity != nil || cleared.StoreID != nil || cleared.Description != nil || cleared.CategoryID != nil {
		t.Fatalf("not cleared: %+v", cleared)
	}

	_, err = f.svc.UpdateItem(f.ctx, f.user, f.household, other.ID, UpdateItemInput{Name: "молоко 2,5%"})
	assertKind(t, err, apperr.KindConflict)

	bad := uuid.Must(uuid.NewV7())
	_, err = f.svc.UpdateItem(f.ctx, f.user, f.household, item.ID, UpdateItemInput{Name: "Молоко", StoreID: &bad})
	assertKind(t, err, apperr.KindInvalid)
}

func TestDeleteStoreDetachesItems(t *testing.T) {
	f := setup(t)
	st, _ := f.svc.CreateStore(f.ctx, f.user, f.household, "Аптека")
	item := f.add(t, AddItemInput{Name: "Аспирин", StoreID: &st.ID})
	if item.StoreID == nil {
		t.Fatal("store not set")
	}
	if err := f.svc.DeleteStore(f.ctx, f.user, f.household, st.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	if len(snap.Stores) != 0 || snap.Items[0].StoreID != nil || snap.Products[0].DefaultStoreID != nil {
		t.Fatalf("store not detached: %+v", snap)
	}
	assertKind(t, f.svc.DeleteStore(f.ctx, f.user, f.household, st.ID), apperr.KindNotFound)
}

func TestCategoriesLifecycle(t *testing.T) {
	f := setup(t)
	emoji := "🍷"
	cat, err := f.svc.CreateCategory(f.ctx, f.user, f.household, "Вино", &emoji)
	if err != nil || cat.SortOrder != 15 {
		t.Fatalf("create: %+v %v", cat, err)
	}
	item := f.add(t, AddItemInput{Name: "Каберне", CategoryID: &cat.ID})
	if *item.CategoryID != cat.ID {
		t.Fatal("explicit category ignored")
	}

	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	ids := make([]uuid.UUID, len(snap.Categories))
	for i, c := range snap.Categories {
		ids[len(ids)-1-i] = c.ID // reverse
	}
	cats, err := f.svc.ReorderCategories(f.ctx, f.user, f.household, ids)
	if err != nil || cats[0].ID != cat.ID {
		t.Fatalf("reorder: first=%v err=%v", cats[0].Name, err)
	}
	_, err = f.svc.ReorderCategories(f.ctx, f.user, f.household, ids[1:])
	assertKind(t, err, apperr.KindInvalid)

	if err := f.svc.DeleteCategory(f.ctx, f.user, f.household, cat.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ = f.svc.Snapshot(f.ctx, f.user, f.household)
	if snap.Items[0].CategoryID != nil || len(snap.Categories) != 14 {
		t.Fatalf("category not detached: %+v", snap.Items[0])
	}
}

func TestVersionGrowsWithEveryWrite(t *testing.T) {
	f := setup(t)
	v0, _ := f.svc.CurrentVersion(f.ctx, f.user, f.household)
	item := f.add(t, AddItemInput{Name: "Яблоки"})
	v1, _ := f.svc.CurrentVersion(f.ctx, f.user, f.household)
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, item.ID, true); err != nil {
		t.Fatal(err)
	}
	v2, _ := f.svc.CurrentVersion(f.ctx, f.user, f.household)
	if !(v0 < v1 && v1 < v2) {
		t.Fatalf("versions must grow: %d %d %d", v0, v1, v2)
	}
	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	if snap.Version != v2 {
		t.Fatalf("snapshot version %d != %d", snap.Version, v2)
	}
}

func TestNonMemberIsRejected(t *testing.T) {
	f := setup(t)
	stranger := uuid.Must(uuid.NewV7())
	_, err := f.svc.Snapshot(f.ctx, stranger, f.household)
	assertKind(t, err, apperr.KindNotFound)
	_, _, err = f.svc.AddItem(f.ctx, stranger, f.household, AddItemInput{Name: "Хлеб"})
	assertKind(t, err, apperr.KindNotFound)
	if len(f.pub.events) != 0 {
		t.Fatal("rejected writes must not publish events")
	}
}

func TestValidation(t *testing.T) {
	f := setup(t)
	neg, bad := -1.0, "bucket"
	cases := []AddItemInput{
		{Name: "   "},
		{Name: "Молоко", Quantity: &neg},
		{Name: "Молоко", Unit: &bad},
		{Name: string(make([]rune, 81))},
	}
	for _, in := range cases {
		_, _, err := f.svc.AddItem(f.ctx, f.user, f.household, in)
		assertKind(t, err, apperr.KindInvalid)
	}
}

func assertKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != kind {
		t.Fatalf("want apperr kind %d, got %v", kind, err)
	}
}
