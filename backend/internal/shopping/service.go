// Package shopping is the shopping list: items, the household's product
// catalog, categories and stores.
package shopping

import (
	"context"
	"errors"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
	"foodmanager/internal/store"
)

var (
	errHouseholdNotFound = apperr.NotFoundCode(apperr.CodeHouseholdNotFound, "семья не найдена")
	errItemNotFound      = apperr.NotFoundCode("item_not_found", "позиция не найдена — возможно, её уже удалили")
	errStoreNotFound     = apperr.NotFound("магазин не найден")
	errCategoryNotFound  = apperr.NotFound("категория не найдена")
)

type Service struct {
	pool *pgxpool.Pool
	q    *store.Queries
	pub  events.Publisher
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, pub events.Publisher) *Service {
	return &Service{pool: pool, q: store.New(pool), pub: pub, now: time.Now}
}

// Snapshot is everything the list screen needs, read consistently as of
// Version.
type Snapshot struct {
	Version    int64
	Items      []store.ListItemViewsRow
	Products   []store.Product
	Categories []store.Category
	Stores     []store.Store
}

func (s *Service) Snapshot(ctx context.Context, userID, householdID uuid.UUID) (Snapshot, error) {
	var snap Snapshot
	// REPEATABLE READ: all five reads see the same moment, so the returned
	// version really describes the returned data.
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, s.pool, opts, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := requireMember(ctx, q, userID, householdID); err != nil {
			return err
		}
		var err error
		if snap.Version, err = q.GetHouseholdVersion(ctx, householdID); err != nil {
			return err
		}
		if snap.Items, err = q.ListItemViews(ctx, store.ListItemViewsParams{HouseholdID: householdID}); err != nil {
			return err
		}
		if snap.Products, err = q.ListProducts(ctx, householdID); err != nil {
			return err
		}
		if snap.Categories, err = q.ListCategories(ctx, householdID); err != nil {
			return err
		}
		snap.Stores, err = q.ListStores(ctx, householdID)
		return err
	})
	return snap, err
}

// Delta is what changed after a given version: live rows that were created
// or updated, plus ids of rows that were deleted.
type Delta struct {
	Version    int64
	Items      []store.ListItemViewsRow
	Products   []store.Product
	Categories []store.Category
	Stores     []store.Store
	Deleted    Tombstones
}

type Tombstones struct {
	Items, Products, Categories, Stores []uuid.UUID
}

// Changes returns everything that changed after version since. A client that
// applies it to its snapshot at `since` ends up exactly at Delta.Version.
func (s *Service) Changes(ctx context.Context, userID, householdID uuid.UUID, since int64) (Delta, error) {
	// Empty (not nil) slices, so the JSON has [] rather than null.
	d := Delta{
		Items: []store.ListItemViewsRow{}, Products: []store.Product{},
		Categories: []store.Category{}, Stores: []store.Store{},
		Deleted: Tombstones{Items: []uuid.UUID{}, Products: []uuid.UUID{}, Categories: []uuid.UUID{}, Stores: []uuid.UUID{}},
	}
	if since < 0 {
		return d, apperr.Invalid("since", "версия не может быть отрицательной")
	}
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, s.pool, opts, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := requireMember(ctx, q, userID, householdID); err != nil {
			return err
		}
		var err error
		if d.Version, err = q.GetHouseholdVersion(ctx, householdID); err != nil {
			return err
		}
		if since >= d.Version {
			return nil // nothing new (or the client is ahead: it will reload)
		}
		if d.Items, err = q.ListItemViews(ctx, store.ListItemViewsParams{HouseholdID: householdID, Since: &since}); err != nil {
			return err
		}
		if d.Products, err = q.ListProductsSince(ctx, store.ListProductsSinceParams{HouseholdID: householdID, Since: since}); err != nil {
			return err
		}
		if d.Categories, err = q.ListCategoriesSince(ctx, store.ListCategoriesSinceParams{HouseholdID: householdID, Since: since}); err != nil {
			return err
		}
		if d.Stores, err = q.ListStoresSince(ctx, store.ListStoresSinceParams{HouseholdID: householdID, Since: since}); err != nil {
			return err
		}
		tombs, err := q.ListTombstonesSince(ctx, store.ListTombstonesSinceParams{HouseholdID: householdID, Since: since})
		if err != nil {
			return err
		}
		for _, t := range tombs {
			switch t.Kind {
			case "item":
				d.Deleted.Items = append(d.Deleted.Items, t.ID)
			case "product":
				d.Deleted.Products = append(d.Deleted.Products, t.ID)
			case "category":
				d.Deleted.Categories = append(d.Deleted.Categories, t.ID)
			case "store":
				d.Deleted.Stores = append(d.Deleted.Stores, t.ID)
			}
		}
		return nil
	})
	return d, err
}

// CurrentVersion returns the household's data version, checking membership.
func (s *Service) CurrentVersion(ctx context.Context, userID, householdID uuid.UUID) (int64, error) {
	if err := requireMember(ctx, s.q, userID, householdID); err != nil {
		return 0, err
	}
	return s.q.GetHouseholdVersion(ctx, householdID)
}

// ---- Items ----

type AddItemInput struct {
	ID         *uuid.UUID
	Name       string
	Quantity   *float64
	Unit       *string
	Note       *string
	StoreID    *uuid.UUID
	CategoryID *uuid.UUID
}

// AddItem puts a product on the list, creating the product on first use.
// created is false when the product was already on the list and the
// existing item was updated instead.
func (s *Service) AddItem(ctx context.Context, userID, householdID uuid.UUID, in AddItemInput) (item store.ListItemViewsRow, created bool, err error) {
	name, err := requiredText(in.Name, maxProductName, "name", "укажите, что купить")
	if err != nil {
		return item, false, err
	}
	note, err := optionalText(in.Note, maxNote, "note")
	if err != nil {
		return item, false, err
	}
	qty, unit, err := normalizeQuantity(in.Quantity, in.Unit)
	if err != nil {
		return item, false, err
	}
	newID := uuid.Must(uuid.NewV7())
	if in.ID != nil {
		newID = *in.ID
	}

	var itemID uuid.UUID
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		// A retried request (e.g. after a flaky connection) must not
		// create a second item.
		existing, err := q.GetItemHousehold(ctx, newID)
		switch {
		case err == nil && existing.HouseholdID == householdID && existing.DeletedAt == nil:
			itemID, created = newID, true
			return nil
		case err == nil:
			return apperr.Conflict("id_taken", "позиция с таким id уже существует")
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		if err := checkRefs(ctx, q, householdID, in.StoreID, in.CategoryID); err != nil {
			return err
		}
		product, err := s.findOrCreateProduct(ctx, q, householdID, name, in.CategoryID, version)
		if err != nil {
			return err
		}

		activeID, err := q.GetActiveItemForProduct(ctx, store.GetActiveItemForProductParams{HouseholdID: householdID, ProductID: product.ID})
		switch {
		case err == nil:
			itemID, created = activeID, false
			return q.MergeItemDetails(ctx, store.MergeItemDetailsParams{
				Quantity: qty, Unit: unit, Note: note, StoreID: in.StoreID,
				Version: version, HouseholdID: householdID, ID: activeID,
			})
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		itemID, created = newID, true
		if err := q.CreateItem(ctx, store.CreateItemParams{
			ID: newID, HouseholdID: householdID, ProductID: product.ID,
			Quantity: qty, Unit: unit, Note: note, StoreID: in.StoreID,
			AddedBy: &userID, Version: version,
		}); err != nil {
			return err
		}
		return q.RecordProductUse(ctx, store.RecordProductUseParams{
			Unit: unit, StoreID: in.StoreID, Version: version, HouseholdID: householdID, ID: product.ID,
		})
	})
	if err != nil {
		return item, false, err
	}
	item, err = s.itemView(ctx, householdID, itemID)
	return item, created, err
}

func (s *Service) findOrCreateProduct(ctx context.Context, q *store.Queries, householdID uuid.UUID, name string, categoryID *uuid.UUID, version int64) (store.Product, error) {
	product, err := q.GetProductByName(ctx, store.GetProductByNameParams{HouseholdID: householdID, Name: name})
	if err == nil {
		if categoryID != nil && (product.CategoryID == nil || *product.CategoryID != *categoryID) {
			if err := q.SetProductCategory(ctx, store.SetProductCategoryParams{
				CategoryID: categoryID, Version: version, HouseholdID: householdID, ID: product.ID,
			}); err != nil {
				return product, err
			}
			err = q.TouchItemsOfProduct(ctx, store.TouchItemsOfProductParams{
				Version: version, HouseholdID: householdID, ProductID: product.ID,
			})
		}
		return product, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return product, err
	}

	if categoryID == nil {
		key := guessCategoryKey(name)
		cat, err := q.GetCategoryByKey(ctx, store.GetCategoryByKeyParams{HouseholdID: householdID, Key: &key})
		if err == nil {
			categoryID = &cat.ID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return product, err
		}
		// The family may have deleted the guessed category: then no category.
	}
	return q.CreateProduct(ctx, store.CreateProductParams{
		HouseholdID: householdID, Name: capitalizeFirst(name), CategoryID: categoryID, Version: version,
	})
}

// capitalizeFirst makes "яйца" look like "Яйца" in the list. Only new
// products are touched: an explicit rename keeps exactly what was typed.
func capitalizeFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

type UpdateItemInput struct {
	Name        string
	Description *string
	CategoryID  *uuid.UUID
	Quantity    *float64
	Unit        *string
	Note        *string
	StoreID     *uuid.UUID
}

// UpdateItem replaces the editable fields of an item and its product.
func (s *Service) UpdateItem(ctx context.Context, userID, householdID, itemID uuid.UUID, in UpdateItemInput) (store.ListItemViewsRow, error) {
	name, err := requiredText(in.Name, maxProductName, "name", "укажите название")
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	description, err := optionalText(in.Description, maxDescription, "description")
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	note, err := optionalText(in.Note, maxNote, "note")
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	qty, unit, err := normalizeQuantity(in.Quantity, in.Unit)
	if err != nil {
		return store.ListItemViewsRow{}, err
	}

	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		productID, err := q.GetItemProduct(ctx, store.GetItemProductParams{HouseholdID: householdID, ID: itemID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errItemNotFound
		}
		if err != nil {
			return err
		}
		if err := checkRefs(ctx, q, householdID, in.StoreID, in.CategoryID); err != nil {
			return err
		}

		// Renaming must not collide with another product of the family.
		other, err := q.GetProductByName(ctx, store.GetProductByNameParams{HouseholdID: householdID, Name: name})
		if err == nil && other.ID != productID {
			return apperr.Conflict("product_exists", "товар «"+other.Name+"» уже есть, выберите другое название")
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if err := q.UpdateProduct(ctx, store.UpdateProductParams{
			Name: name, Description: description, CategoryID: in.CategoryID,
			Version: version, HouseholdID: householdID, ID: productID,
		}); err != nil {
			return err
		}
		// Other items of this product (e.g. bought ones) show its name too.
		if err := q.TouchItemsOfProduct(ctx, store.TouchItemsOfProductParams{
			Version: version, HouseholdID: householdID, ProductID: productID,
		}); err != nil {
			return err
		}
		return q.UpdateItemDetails(ctx, store.UpdateItemDetailsParams{
			Quantity: qty, Unit: unit, Note: note, StoreID: in.StoreID,
			Version: version, HouseholdID: householdID, ID: itemID,
		})
	})
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	return s.itemView(ctx, householdID, itemID)
}

func (s *Service) SetChecked(ctx context.Context, userID, householdID, itemID uuid.UUID, checked bool) (store.ListItemViewsRow, error) {
	err := s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		productID, err := q.GetItemProduct(ctx, store.GetItemProductParams{HouseholdID: householdID, ID: itemID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errItemNotFound
		}
		if err != nil {
			return err
		}
		params := store.SetItemCheckedParams{Version: version, HouseholdID: householdID, ID: itemID}
		now := s.now()
		if checked {
			params.CheckedAt, params.CheckedBy = &now, &userID
		} else {
			// Unchecking would put the product on the list twice if someone
			// has added it again in the meantime.
			activeID, err := q.GetActiveItemForProduct(ctx, store.GetActiveItemForProductParams{HouseholdID: householdID, ProductID: productID})
			if err == nil && activeID != itemID {
				return apperr.Conflict("already_on_list", "этот товар уже снова есть в списке")
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if _, err := q.SetItemChecked(ctx, params); err != nil {
			return err
		}
		if checked {
			return recordPurchase(ctx, q, householdID, itemID, productID, userID, now, version)
		}
		return unrecordPurchase(ctx, q, householdID, itemID, productID, version)
	})
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	return s.itemView(ctx, householdID, itemID)
}

func (s *Service) DeleteItem(ctx context.Context, userID, householdID, itemID uuid.UUID) error {
	return s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		n, err := q.SoftDeleteItem(ctx, store.SoftDeleteItemParams{Version: version, HouseholdID: householdID, ID: itemID})
		if err == nil && n == 0 {
			return errItemNotFound
		}
		return err
	})
}

// ClearChecked removes all bought items from the list.
func (s *Service) ClearChecked(ctx context.Context, userID, householdID uuid.UUID) (int64, error) {
	var removed int64
	err := s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		var err error
		removed, err = q.SoftDeleteCheckedItems(ctx, store.SoftDeleteCheckedItemsParams{Version: version, HouseholdID: householdID})
		return err
	})
	return removed, err
}

func (s *Service) itemView(ctx context.Context, householdID, itemID uuid.UUID) (store.ListItemViewsRow, error) {
	rows, err := s.q.ListItemViews(ctx, store.ListItemViewsParams{HouseholdID: householdID, ItemID: &itemID})
	if err != nil {
		return store.ListItemViewsRow{}, err
	}
	if len(rows) == 0 {
		return store.ListItemViewsRow{}, errItemNotFound
	}
	return rows[0], nil
}

// ---- Stores ----

func (s *Service) CreateStore(ctx context.Context, userID, householdID uuid.UUID, name string) (store.Store, error) {
	name, err := requiredText(name, maxStoreName, "name", "укажите название магазина")
	if err != nil {
		return store.Store{}, err
	}
	var st store.Store
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		st, err = q.CreateStore(ctx, store.CreateStoreParams{HouseholdID: householdID, Name: name, Version: version})
		return err
	})
	return st, err
}

func (s *Service) RenameStore(ctx context.Context, userID, householdID, storeID uuid.UUID, name string) (store.Store, error) {
	name, err := requiredText(name, maxStoreName, "name", "укажите название магазина")
	if err != nil {
		return store.Store{}, err
	}
	var st store.Store
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		st, err = q.RenameStore(ctx, store.RenameStoreParams{Name: name, Version: version, HouseholdID: householdID, ID: storeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStoreNotFound
		}
		return err
	})
	return st, err
}

// DeleteStore removes the store; items and products assigned to it stay,
// just without a store.
func (s *Service) DeleteStore(ctx context.Context, userID, householdID, storeID uuid.UUID) error {
	return s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		n, err := q.SoftDeleteStore(ctx, store.SoftDeleteStoreParams{Version: version, HouseholdID: householdID, ID: storeID})
		if err != nil {
			return err
		}
		if n == 0 {
			return errStoreNotFound
		}
		ref := &storeID
		if err := q.DetachStoreFromItems(ctx, store.DetachStoreFromItemsParams{Version: version, HouseholdID: householdID, StoreID: ref}); err != nil {
			return err
		}
		return q.DetachStoreFromProducts(ctx, store.DetachStoreFromProductsParams{Version: version, HouseholdID: householdID, StoreID: ref})
	})
}

// ---- Categories ----

func (s *Service) CreateCategory(ctx context.Context, userID, householdID uuid.UUID, name string, emoji *string) (store.Category, error) {
	name, err := requiredText(name, maxCategoryName, "name", "укажите название категории")
	if err != nil {
		return store.Category{}, err
	}
	emoji, err = optionalText(emoji, maxEmoji, "emoji")
	if err != nil {
		return store.Category{}, err
	}
	var cat store.Category
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		order, err := q.NextCategorySortOrder(ctx, householdID)
		if err != nil {
			return err
		}
		cat, err = q.CreateCategory(ctx, store.CreateCategoryParams{
			HouseholdID: householdID, Name: name, Emoji: emoji, SortOrder: order, Version: version,
		})
		return err
	})
	return cat, err
}

func (s *Service) UpdateCategory(ctx context.Context, userID, householdID, categoryID uuid.UUID, name string, emoji *string) (store.Category, error) {
	name, err := requiredText(name, maxCategoryName, "name", "укажите название категории")
	if err != nil {
		return store.Category{}, err
	}
	emoji, err = optionalText(emoji, maxEmoji, "emoji")
	if err != nil {
		return store.Category{}, err
	}
	var cat store.Category
	err = s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		cat, err = q.UpdateCategory(ctx, store.UpdateCategoryParams{
			Name: name, Emoji: emoji, Version: version, HouseholdID: householdID, ID: categoryID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCategoryNotFound
		}
		return err
	})
	return cat, err
}

// DeleteCategory removes the category; its products become uncategorized.
func (s *Service) DeleteCategory(ctx context.Context, userID, householdID, categoryID uuid.UUID) error {
	return s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		n, err := q.SoftDeleteCategory(ctx, store.SoftDeleteCategoryParams{Version: version, HouseholdID: householdID, ID: categoryID})
		if err != nil {
			return err
		}
		if n == 0 {
			return errCategoryNotFound
		}
		// Bump items first: after detaching we could no longer find them.
		if err := q.TouchItemsOfCategory(ctx, store.TouchItemsOfCategoryParams{
			Version: version, HouseholdID: householdID, CategoryID: &categoryID,
		}); err != nil {
			return err
		}
		return q.DetachCategoryFromProducts(ctx, store.DetachCategoryFromProductsParams{
			Version: version, HouseholdID: householdID, CategoryID: &categoryID,
		})
	})
}

// ReorderCategories sets the order of all categories. ids must list every
// live category exactly once, so concurrent edits can't silently lose one.
func (s *Service) ReorderCategories(ctx context.Context, userID, householdID uuid.UUID, ids []uuid.UUID) ([]store.Category, error) {
	var cats []store.Category
	err := s.write(ctx, userID, householdID, func(q *store.Queries, version int64) error {
		current, err := q.ListCategories(ctx, householdID)
		if err != nil {
			return err
		}
		if !sameIDs(current, ids) {
			return apperr.Invalid("ids", "список категорий изменился, обновите страницу")
		}
		for i, id := range ids {
			if _, err := q.SetCategorySortOrder(ctx, store.SetCategorySortOrderParams{
				SortOrder: int32(i + 1), Version: version, HouseholdID: householdID, ID: id,
			}); err != nil {
				return err
			}
		}
		cats, err = q.ListCategories(ctx, householdID)
		return err
	})
	return cats, err
}

func sameIDs(cats []store.Category, ids []uuid.UUID) bool {
	if len(cats) != len(ids) {
		return false
	}
	want := make(map[uuid.UUID]bool, len(cats))
	for _, c := range cats {
		want[c.ID] = true
	}
	for _, id := range ids {
		if !want[id] {
			return false
		}
		delete(want, id) // catches duplicates in ids
	}
	return true
}

// ---- helpers ----

// write runs fn in a transaction after checking membership and bumping the
// household version, then notifies subscribers once the change is committed.
func (s *Service) write(ctx context.Context, userID, householdID uuid.UUID, fn func(q *store.Queries, version int64) error) error {
	var version int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := requireMember(ctx, q, userID, householdID); err != nil {
			return err
		}
		var err error
		if version, err = q.BumpHouseholdVersion(ctx, householdID); err != nil {
			return err
		}
		return fn(q, version)
	})
	if err == nil {
		s.pub.Publish(householdID, events.Event{Kind: events.KindList, Version: version})
	}
	return err
}

func requireMember(ctx context.Context, q *store.Queries, userID, householdID uuid.UUID) error {
	_, err := q.GetMemberRole(ctx, store.GetMemberRoleParams{HouseholdID: householdID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errHouseholdNotFound
	}
	return err
}

// checkRefs verifies that referenced store and category exist in this
// household. The composite FKs would reject foreign ids anyway; this turns
// that into a readable error and also rejects soft-deleted ones.
func checkRefs(ctx context.Context, q *store.Queries, householdID uuid.UUID, storeID, categoryID *uuid.UUID) error {
	if storeID != nil {
		if _, err := q.GetStore(ctx, store.GetStoreParams{HouseholdID: householdID, ID: *storeID}); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("storeId", "магазин не найден — возможно, его удалили")
		} else if err != nil {
			return err
		}
	}
	if categoryID != nil {
		if _, err := q.GetCategory(ctx, store.GetCategoryParams{HouseholdID: householdID, ID: *categoryID}); errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("categoryId", "категория не найдена — возможно, её удалили")
		} else if err != nil {
			return err
		}
	}
	return nil
}
