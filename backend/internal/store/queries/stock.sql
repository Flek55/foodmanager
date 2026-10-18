-- ---- Home stock ----

-- name: SetProductStock :exec
UPDATE products
SET stock_status = sqlc.narg(status), stock_updated_at = now(), stock_updated_by = sqlc.narg(updated_by),
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id;

-- A purchase restocks a tracked product (untracked ones stay untracked).
-- name: RestockIfTracked :exec
UPDATE products
SET stock_status = 'ok', stock_updated_at = now(), stock_updated_by = @updated_by,
    version = @version, updated_at = now()
WHERE household_id = @household_id AND id = @id AND stock_status IS NOT NULL AND stock_status <> 'ok';

-- name: CreateProductWithID :one
INSERT INTO products (id, household_id, name, category_id, version)
VALUES (@id, @household_id, @name, sqlc.narg(category_id), @version)
RETURNING *;

-- ---- Purchase history ----

-- name: RecordPurchase :exec
INSERT INTO purchase_events (household_id, product_id, item_id, quantity, unit, store_id, bought_by, bought_at)
SELECT i.household_id, i.product_id, i.id, i.quantity, i.unit, i.store_id, @bought_by::uuid, @bought_at::timestamptz
FROM list_items AS i
WHERE i.household_id = @household_id AND i.id = @item_id
ON CONFLICT (item_id) DO NOTHING;

-- name: DeletePurchaseOfItem :exec
DELETE FROM purchase_events WHERE household_id = @household_id AND item_id = @item_id;

-- Recomputes the product's purchase counters from the events.
-- name: RefreshPurchaseStats :exec
UPDATE products AS p
SET purchase_count = s.cnt, last_purchased_at = s.last, version = @version, updated_at = now()
FROM (
    SELECT count(*)::int AS cnt, max(e.bought_at) AS last
    FROM purchase_events AS e
    WHERE e.household_id = @household_id::uuid AND e.product_id = @product_id::uuid
) AS s
WHERE p.household_id = @household_id::uuid AND p.id = @product_id::uuid;

-- name: ListPurchases :many
SELECT e.id, e.bought_at, e.quantity, e.unit, e.store_id, u.display_name AS bought_by_name
FROM purchase_events AS e
LEFT JOIN users AS u ON u.id = e.bought_by
WHERE e.household_id = @household_id AND e.product_id = @product_id
ORDER BY e.bought_at DESC
LIMIT @max_rows;

-- ---- Clearing bought items ----

-- name: HouseholdsWithCheckedBefore :many
SELECT DISTINCT household_id FROM list_items
WHERE checked_at < @cutoff AND deleted_at IS NULL;

-- name: SoftDeleteCheckedBefore :execrows
UPDATE list_items
SET deleted_at = now(), version = @version, updated_at = now()
WHERE household_id = @household_id AND checked_at < @cutoff AND deleted_at IS NULL;
