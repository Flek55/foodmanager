-- Every write to household data first bumps the household version. Besides
-- versioning rows for sync, this takes a row lock on the household, so
-- writes within one family are serialized (no get-or-create races).
-- name: BumpHouseholdVersion :one
UPDATE households SET version = version + 1 WHERE id = @id RETURNING version;

-- name: GetHouseholdVersion :one
SELECT version FROM households WHERE id = @id;

-- ---- Categories ----

-- name: CreateCategory :one
INSERT INTO categories (household_id, key, name, emoji, sort_order, version)
VALUES (@household_id, sqlc.narg(key), @name, sqlc.narg(emoji), @sort_order, @version)
RETURNING *;

-- name: ListCategories :many
SELECT * FROM categories
WHERE household_id = @household_id AND deleted_at IS NULL
ORDER BY sort_order, name;

-- name: GetCategory :one
SELECT * FROM categories
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: GetCategoryByKey :one
SELECT * FROM categories
WHERE household_id = @household_id AND key = @key AND deleted_at IS NULL;

-- name: NextCategorySortOrder :one
SELECT (COALESCE(max(sort_order), 0) + 1)::int FROM categories
WHERE household_id = @household_id AND deleted_at IS NULL;

-- name: UpdateCategory :one
UPDATE categories
SET name = @name, emoji = sqlc.narg(emoji), version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SetCategorySortOrder :execrows
UPDATE categories
SET sort_order = @sort_order, version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCategory :execrows
UPDATE categories
SET deleted_at = now(), version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: DetachCategoryFromProducts :exec
UPDATE products
SET category_id = NULL, version = @version, updated_at = now()
WHERE household_id = @household_id AND category_id = @category_id;

-- ---- Stores ----

-- name: CreateStore :one
INSERT INTO stores (household_id, name, sort_order, version)
VALUES (
    @household_id, @name,
    (SELECT COALESCE(max(s.sort_order), 0) + 1 FROM stores s
     WHERE s.household_id = @household_id AND s.deleted_at IS NULL),
    @version
)
RETURNING *;

-- name: ListStores :many
SELECT * FROM stores
WHERE household_id = @household_id AND deleted_at IS NULL
ORDER BY sort_order, name;

-- name: GetStore :one
SELECT * FROM stores
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: RenameStore :one
UPDATE stores
SET name = @name, version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteStore :execrows
UPDATE stores
SET deleted_at = now(), version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: DetachStoreFromItems :exec
UPDATE list_items
SET store_id = NULL, version = @version, updated_at = now()
WHERE household_id = @household_id AND store_id = @store_id;

-- name: DetachStoreFromProducts :exec
UPDATE products
SET default_store_id = NULL, version = @version, updated_at = now()
WHERE household_id = @household_id AND default_store_id = @store_id;

-- ---- Products ----

-- name: ListProducts :many
SELECT * FROM products
WHERE household_id = @household_id AND deleted_at IS NULL
ORDER BY use_count DESC, name;

-- name: GetProduct :one
SELECT * FROM products
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: GetProductByName :one
SELECT * FROM products
WHERE household_id = @household_id AND lower(name) = lower(@name::text) AND deleted_at IS NULL;

-- name: CreateProduct :one
INSERT INTO products (household_id, name, category_id, version)
VALUES (@household_id, @name, sqlc.narg(category_id), @version)
RETURNING *;

-- name: UpdateProduct :exec
UPDATE products
SET name = @name, description = sqlc.narg(description), category_id = sqlc.narg(category_id),
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id;

-- Called whenever the product is put on the list: ranks suggestions and
-- remembers the last used unit and store.
-- name: RecordProductUse :exec
UPDATE products
SET use_count = use_count + 1,
    default_unit = COALESCE(sqlc.narg(unit), default_unit),
    default_store_id = COALESCE(sqlc.narg(store_id), default_store_id),
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id;

-- name: SetProductCategory :exec
UPDATE products
SET category_id = @category_id, version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id;

-- ---- List items ----

-- Looks an item up by id across all households: used to make a retried
-- create idempotent and to reject ids that belong to someone else.
-- name: GetItemHousehold :one
SELECT household_id, deleted_at FROM list_items WHERE id = @id;

-- name: GetActiveItemForProduct :one
SELECT id FROM list_items
WHERE household_id = @household_id AND product_id = @product_id
  AND checked_at IS NULL AND deleted_at IS NULL;

-- name: CreateItem :exec
INSERT INTO list_items (id, household_id, product_id, quantity, unit, note, store_id, added_by, version)
VALUES (@id, @household_id, @product_id, sqlc.narg(quantity), sqlc.narg(unit), sqlc.narg(note),
        sqlc.narg(store_id), @added_by, @version);

-- name: GetItemProduct :one
SELECT product_id FROM list_items
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: UpdateItemDetails :exec
UPDATE list_items
SET quantity = sqlc.narg(quantity), unit = sqlc.narg(unit), note = sqlc.narg(note),
    store_id = sqlc.narg(store_id), version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- Merging a re-added product into its existing item: only fields that were
-- actually given overwrite the old values.
-- name: MergeItemDetails :exec
UPDATE list_items
SET quantity = COALESCE(sqlc.narg(quantity), quantity),
    unit = CASE WHEN sqlc.narg(quantity)::double precision IS NULL THEN unit ELSE sqlc.narg(unit) END,
    note = COALESCE(sqlc.narg(note), note),
    store_id = COALESCE(sqlc.narg(store_id), store_id),
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id;

-- name: SetItemChecked :execrows
UPDATE list_items
SET checked_at = sqlc.narg(checked_at), checked_by = sqlc.narg(checked_by),
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: SoftDeleteItem :execrows
UPDATE list_items
SET deleted_at = now(), version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND deleted_at IS NULL;

-- name: SoftDeleteCheckedItems :execrows
UPDATE list_items
SET deleted_at = now(), version = @version, updated_at = now()
WHERE household_id = @household_id AND checked_at IS NOT NULL AND deleted_at IS NULL;

-- Items as the UI shows them: joined with their product and people's names.
-- Pass item_id to fetch a single item, or NULL for the whole list.
-- name: ListItemViews :many
SELECT
    i.id, i.product_id, p.name, p.description, p.category_id,
    i.quantity, i.unit, i.note, i.store_id,
    au.display_name AS added_by_name,
    i.created_at, i.checked_at,
    cu.display_name AS checked_by_name
FROM list_items i
JOIN products p ON p.id = i.product_id
LEFT JOIN users au ON au.id = i.added_by
LEFT JOIN users cu ON cu.id = i.checked_by
WHERE i.household_id = @household_id
  AND i.deleted_at IS NULL
  AND (sqlc.narg(item_id)::uuid IS NULL OR i.id = sqlc.narg(item_id)::uuid)
ORDER BY i.created_at, i.id;
