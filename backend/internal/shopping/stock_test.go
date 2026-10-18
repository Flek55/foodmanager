package shopping

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"foodmanager/internal/apperr"
	"foodmanager/internal/store"
)

func TestTrackProductAndRunningOut(t *testing.T) {
	f := setup(t)
	productID, itemID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	res, err := f.svc.TrackProduct(f.ctx, f.user, f.household, "туалетная бумага", StockOK, &productID, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := res.Product
	if p.ID != productID || p.Name != "Туалетная бумага" || *p.StockStatus != StockOK || res.Item != nil {
		t.Fatalf("track: %+v item=%v", p, res.Item)
	}
	if f.categoryKey(t, p.CategoryID) != "household" {
		t.Fatalf("category not guessed: %v", f.categoryKey(t, p.CategoryID))
	}

	// Running out puts it on the list, under the client's item id.
	res, err = f.svc.SetStock(f.ctx, f.user, f.household, productID, StockOut, &itemID)
	if err != nil {
		t.Fatal(err)
	}
	if *res.Product.StockStatus != StockOut || res.Item == nil || res.Item.ID != itemID {
		t.Fatalf("out: %+v item=%+v", res.Product.StockStatus, res.Item)
	}

	// Already on the list: no second item; the existing one is returned, even
	// when the client proposes a different id (it then re-points to ours).
	res, err = f.svc.SetStock(f.ctx, f.user, f.household, productID, StockLow, &itemID)
	if err != nil || res.Item == nil || res.Item.ID != itemID {
		t.Fatalf("retry while on list: item=%+v err=%v", res.Item, err)
	}
	otherItem := uuid.Must(uuid.NewV7())
	res, err = f.svc.SetStock(f.ctx, f.user, f.household, productID, StockOut, &otherItem)
	if err != nil || res.Item == nil || res.Item.ID != itemID {
		t.Fatalf("other id while on list: item=%+v err=%v", res.Item, err)
	}
	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	if len(snap.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(snap.Items))
	}

	// Tracking a name that exists returns the existing product.
	other := uuid.Must(uuid.NewV7())
	res, err = f.svc.TrackProduct(f.ctx, f.user, f.household, "Туалетная Бумага", StockOK, &other, nil)
	if err != nil || res.Product.ID != productID {
		t.Fatalf("existing name: id=%v err=%v", res.Product.ID, err)
	}

	res, err = f.svc.SetStock(f.ctx, f.user, f.household, productID, StockUntracked, nil)
	if err != nil || res.Product.StockStatus != nil {
		t.Fatalf("untrack: %+v %v", res.Product.StockStatus, err)
	}
}

func TestBuyingRecordsHistoryAndRestocks(t *testing.T) {
	f := setup(t)
	itemID := uuid.Must(uuid.NewV7())
	res, err := f.svc.TrackProduct(f.ctx, f.user, f.household, "Кофе", StockOut, nil, &itemID)
	if err != nil || res.Item == nil {
		t.Fatalf("track out: %+v %v", res.Item, err)
	}
	productID := res.Product.ID

	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, itemID, true); err != nil {
		t.Fatal(err)
	}
	p := f.product(t, productID)
	if *p.StockStatus != StockOK || p.PurchaseCount != 1 || p.LastPurchasedAt == nil {
		t.Fatalf("after buying: status=%v count=%d last=%v", *p.StockStatus, p.PurchaseCount, p.LastPurchasedAt)
	}

	// Checking twice doesn't double-count.
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, itemID, true); err != nil {
		t.Fatal(err)
	}
	history, err := f.svc.Purchases(f.ctx, f.user, f.household, productID)
	if err != nil || len(history) != 1 || *history[0].BoughtByName != "Маша" {
		t.Fatalf("history: %+v %v", history, err)
	}

	// Unchecking (it was a mistake) removes the purchase again.
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, itemID, false); err != nil {
		t.Fatal(err)
	}
	if p := f.product(t, productID); p.PurchaseCount != 0 || p.LastPurchasedAt != nil {
		t.Fatalf("after uncheck: count=%d last=%v", p.PurchaseCount, p.LastPurchasedAt)
	}

	// Untracked products get history too, but don't start being tracked.
	bread := f.add(t, AddItemInput{Name: "Хлеб"})
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, bread.ID, true); err != nil {
		t.Fatal(err)
	}
	if p := f.product(t, bread.ProductID); p.StockStatus != nil || p.PurchaseCount != 1 {
		t.Fatalf("untracked bought: status=%v count=%d", p.StockStatus, p.PurchaseCount)
	}
}

func TestClearOldBoughtItems(t *testing.T) {
	f := setup(t)
	old := f.add(t, AddItemInput{Name: "Молоко"})
	fresh := f.add(t, AddItemInput{Name: "Хлеб"})
	open := f.add(t, AddItemInput{Name: "Сыр"})

	f.svc.now = func() time.Time { return time.Now().Add(-13 * time.Hour) }
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, old.ID, true); err != nil {
		t.Fatal(err)
	}
	f.svc.now = time.Now
	if _, err := f.svc.SetChecked(f.ctx, f.user, f.household, fresh.ID, true); err != nil {
		t.Fatal(err)
	}
	events := len(f.pub.events)

	n, err := f.svc.ClearOldBoughtItems(f.ctx, BoughtItemsTTL)
	if err != nil || n != 1 {
		t.Fatalf("cleared %d, err %v", n, err)
	}
	snap, _ := f.svc.Snapshot(f.ctx, f.user, f.household)
	ids := map[uuid.UUID]bool{}
	for _, it := range snap.Items {
		ids[it.ID] = true
	}
	if ids[old.ID] || !ids[fresh.ID] || !ids[open.ID] {
		t.Fatalf("wrong items left: %v", ids)
	}
	if len(f.pub.events) != events+1 {
		t.Fatal("clearing must notify clients")
	}
	// The purchase history survives the cleanup.
	if p := f.product(t, old.ProductID); p.PurchaseCount != 1 {
		t.Fatalf("history lost: count=%d", p.PurchaseCount)
	}
}

func TestStockValidation(t *testing.T) {
	f := setup(t)
	_, err := f.svc.TrackProduct(f.ctx, f.user, f.household, "Кофе", "plenty", nil, nil)
	assertKind(t, err, apperr.KindInvalid)
	_, err = f.svc.TrackProduct(f.ctx, f.user, f.household, "Кофе", StockUntracked, nil, nil)
	assertKind(t, err, apperr.KindInvalid)
	_, err = f.svc.SetStock(f.ctx, f.user, f.household, uuid.Must(uuid.NewV7()), StockOut, nil)
	assertKind(t, err, apperr.KindNotFound)
	_, err = f.svc.Purchases(f.ctx, uuid.Must(uuid.NewV7()), f.household, uuid.Must(uuid.NewV7()))
	assertKind(t, err, apperr.KindNotFound)
}

func (f fixture) product(t *testing.T, id uuid.UUID) store.Product {
	t.Helper()
	p, err := store.New(f.pool).GetProduct(f.ctx, store.GetProductParams{HouseholdID: f.household, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
