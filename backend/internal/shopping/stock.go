package shopping

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
	"foodmanager/internal/store"
)

const (
	StockOK        = "ok"
	StockLow       = "low"
	StockOut       = "out"
	StockUntracked = "untracked"

	// BoughtItemsTTL: bought items leave the list this long after check-off
	// (their purchase stays in the history).
	BoughtItemsTTL = 12 * time.Hour

	maxPurchasesShown = 20
)

var errProductNotFound = apperr.NotFoundCode("product_not_found", "товар не найден")

// StockResult is a product after a stock change, plus — for low/out — its
// open item on the shopping list (just added or already there).
type StockResult struct {
	Product store.Product
	Item    *store.ListItemViewsRow
}

// SetStock marks how much of a product is left at home. "low" and "out"
// put the product on the shopping list (as itemID, if given — offline
// clients pass their own id) unless it's already there. "untracked" stops
// tracking the product at home.
func (s *Service) SetStock(ctx context.Context, userID, householdID, productID uuid.UUID, status string, itemID *uuid.UUID) (StockResult, error) {
	if err := validateStockStatus(status); err != nil {
		return StockResult{}, err
	}
	var addedItem *uuid.UUID
	err := s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		if _, err := q.GetProduct(ctx, store.GetProductParams{HouseholdID: householdID, ID: productID}); errors.Is(err, pgx.ErrNoRows) {
			return errProductNotFound
		} else if err != nil {
			return err
		}
		var err error
		addedItem, err = applyStock(ctx, q, householdID, productID, userID, status, itemID, version)
		return err
	})
	if err != nil {
		return StockResult{}, err
	}
	return s.stockResult(ctx, householdID, productID, addedItem)
}

// TrackProduct starts tracking a product at home by name, creating the
// product if the family never had it (as productID, if given). Returns the
// existing product when the name is already known.
func (s *Service) TrackProduct(ctx context.Context, userID, householdID uuid.UUID, name, status string, productID, itemID *uuid.UUID) (StockResult, error) {
	name, err := requiredText(name, maxProductName, "name", "укажите, что отслеживать")
	if err != nil {
		return StockResult{}, err
	}
	if status == StockUntracked {
		return StockResult{}, apperr.Invalid("status", "новый товар нельзя сразу убрать из отслеживания")
	}
	if err := validateStockStatus(status); err != nil {
		return StockResult{}, err
	}

	var id uuid.UUID
	var addedItem *uuid.UUID
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		product, err := q.GetProductByName(ctx, store.GetProductByNameParams{HouseholdID: householdID, Name: name})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			newID := uuid.Must(uuid.NewV7())
			if productID != nil {
				newID = *productID
			}
			key := guessCategoryKey(name)
			var categoryID *uuid.UUID
			if cat, err := q.GetCategoryByKey(ctx, store.GetCategoryByKeyParams{HouseholdID: householdID, Key: &key}); err == nil {
				categoryID = &cat.ID
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			product, err = q.CreateProductWithID(ctx, store.CreateProductWithIDParams{
				ID: newID, HouseholdID: householdID, Name: capitalizeFirst(name), CategoryID: categoryID, Version: version,
			})
			if isUniqueViolation(err) {
				return apperr.Conflict("id_taken", "товар с таким id уже существует")
			}
			if err != nil {
				return err
			}
		case err != nil:
			return err
		}
		id = product.ID
		addedItem, err = applyStock(ctx, q, householdID, product.ID, userID, status, itemID, version)
		return err
	})
	if err != nil {
		return StockResult{}, err
	}
	return s.stockResult(ctx, householdID, id, addedItem)
}

// applyStock writes the status and, for low/out, makes sure the product is
// on the list. Returns the id of the product's open list item for low/out —
// newly added or already there — so offline clients can re-point their own
// temporary item id to it.
func applyStock(ctx context.Context, q *store.Queries, householdID, productID, userID uuid.UUID, status string, itemID *uuid.UUID, version int64) (*uuid.UUID, error) {
	var stored *string
	if status != StockUntracked {
		stored = &status
	}
	if err := q.SetProductStock(ctx, store.SetProductStockParams{
		Status: stored, UpdatedBy: &userID, Version: version, HouseholdID: householdID, ID: productID,
	}); err != nil {
		return nil, err
	}
	if status != StockLow && status != StockOut {
		return nil, nil
	}

	activeID, err := q.GetActiveItemForProduct(ctx, store.GetActiveItemForProductParams{HouseholdID: householdID, ProductID: productID})
	if err == nil {
		return &activeID, nil // already on the list (maybe our own retried request)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	newID := uuid.Must(uuid.NewV7())
	if itemID != nil {
		newID = *itemID
	}
	if _, err := q.GetItemHousehold(ctx, newID); err == nil {
		// The id exists but isn't an open item of this product here: it
		// belongs to another family, or was already bought/deleted.
		return nil, apperr.Conflict("id_taken", "позиция с таким id уже существует")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := q.CreateItem(ctx, store.CreateItemParams{
		ID: newID, HouseholdID: householdID, ProductID: productID, AddedBy: &userID, Version: version,
	}); err != nil {
		return nil, err
	}
	if err := q.RecordProductUse(ctx, store.RecordProductUseParams{Version: version, HouseholdID: householdID, ID: productID}); err != nil {
		return nil, err
	}
	return &newID, nil
}

func (s *Service) stockResult(ctx context.Context, householdID, productID uuid.UUID, itemID *uuid.UUID) (StockResult, error) {
	product, err := s.q.GetProduct(ctx, store.GetProductParams{HouseholdID: householdID, ID: productID})
	if err != nil {
		return StockResult{}, err
	}
	res := StockResult{Product: product}
	if itemID != nil {
		item, err := s.itemView(ctx, householdID, *itemID)
		if err != nil {
			return StockResult{}, err
		}
		res.Item = &item
	}
	return res, nil
}

func validateStockStatus(status string) error {
	switch status {
	case StockOK, StockLow, StockOut, StockUntracked:
		return nil
	}
	return apperr.Invalid("status", "неизвестный статус запаса")
}

// recordPurchase is called when an item is checked off: it logs the
// purchase, refreshes the product's counters and restocks it at home.
func recordPurchase(ctx context.Context, q *store.Queries, householdID, itemID, productID, userID uuid.UUID, at time.Time, version int64) error {
	if err := q.RecordPurchase(ctx, store.RecordPurchaseParams{
		BoughtBy: userID, BoughtAt: at, HouseholdID: householdID, ItemID: itemID,
	}); err != nil {
		return err
	}
	if err := q.RefreshPurchaseStats(ctx, store.RefreshPurchaseStatsParams{Version: version, HouseholdID: householdID, ProductID: productID}); err != nil {
		return err
	}
	return q.RestockIfTracked(ctx, store.RestockIfTrackedParams{
		UpdatedBy: &userID, Version: version, HouseholdID: householdID, ID: productID,
	})
}

// unrecordPurchase undoes recordPurchase when an item is unchecked. The
// stock status is left alone: we can't know what it was before.
func unrecordPurchase(ctx context.Context, q *store.Queries, householdID, itemID, productID uuid.UUID, version int64) error {
	if err := q.DeletePurchaseOfItem(ctx, store.DeletePurchaseOfItemParams{HouseholdID: householdID, ItemID: &itemID}); err != nil {
		return err
	}
	return q.RefreshPurchaseStats(ctx, store.RefreshPurchaseStatsParams{Version: version, HouseholdID: householdID, ProductID: productID})
}

type Purchase = store.ListPurchasesRow

// Purchases returns the latest purchases of a product, newest first.
func (s *Service) Purchases(ctx context.Context, userID, householdID, productID uuid.UUID) ([]Purchase, error) {
	if err := requireMember(ctx, s.q, userID, householdID); err != nil {
		return nil, err
	}
	if _, err := s.q.GetProduct(ctx, store.GetProductParams{HouseholdID: householdID, ID: productID}); errors.Is(err, pgx.ErrNoRows) {
		return nil, errProductNotFound
	} else if err != nil {
		return nil, err
	}
	return s.q.ListPurchases(ctx, store.ListPurchasesParams{HouseholdID: householdID, ProductID: productID, MaxRows: maxPurchasesShown})
}

// ClearOldBoughtItems takes items bought more than `age` ago off every
// household's list. Meant for a periodic background job.
func (s *Service) ClearOldBoughtItems(ctx context.Context, age time.Duration) (int64, error) {
	cutoff := s.now().Add(-age)
	households, err := s.q.HouseholdsWithCheckedBefore(ctx, &cutoff)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, householdID := range households {
		var version, n int64
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			q := s.q.WithTx(tx)
			var err error
			if version, err = q.BumpHouseholdVersion(ctx, householdID); err != nil {
				return err
			}
			n, err = q.SoftDeleteCheckedBefore(ctx, store.SoftDeleteCheckedBeforeParams{
				Version: version, HouseholdID: householdID, Cutoff: &cutoff,
			})
			return err
		})
		if err != nil {
			return total, err
		}
		total += n
		s.pub.Publish(householdID, events.Event{Kind: events.KindList, Version: version})
	}
	return total, nil
}
