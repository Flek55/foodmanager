-- Home stock and purchase history.
--
-- Stock lives on the product (one status per product), so it travels with
-- products through delta sync and the offline queue for free.
-- Purchase events are the raw history the "time to buy" forecast (stage 8)
-- will learn from; products keep denormalized counters for the UI.

-- +goose Up
ALTER TABLE products
    -- NULL = the family doesn't track this product at home.
    ADD COLUMN stock_status      text CHECK (stock_status IN ('ok', 'low', 'out')),
    ADD COLUMN stock_updated_at  timestamptz,
    ADD COLUMN stock_updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN purchase_count    int NOT NULL DEFAULT 0,
    ADD COLUMN last_purchased_at timestamptz;

CREATE TABLE purchase_events (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    household_id uuid        NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    product_id   uuid        NOT NULL,
    -- The list item whose check-off recorded this purchase: unchecking it
    -- removes the event again. Unique, so a repeated check is a no-op.
    item_id      uuid UNIQUE,
    quantity     double precision,
    unit         text,
    store_id     uuid,
    bought_by    uuid        REFERENCES users (id) ON DELETE SET NULL,
    bought_at    timestamptz NOT NULL,
    FOREIGN KEY (household_id, product_id) REFERENCES products (household_id, id),
    FOREIGN KEY (household_id, store_id) REFERENCES stores (household_id, id)
);
CREATE INDEX purchase_events_product_idx ON purchase_events (household_id, product_id, bought_at DESC);

-- For the job that clears bought items off the list after a while.
CREATE INDEX list_items_checked_idx ON list_items (checked_at)
    WHERE checked_at IS NOT NULL AND deleted_at IS NULL;

-- History for items bought before this migration.
INSERT INTO purchase_events (household_id, product_id, item_id, quantity, unit, store_id, bought_by, bought_at)
SELECT household_id, product_id, id, quantity, unit, store_id, checked_by, checked_at
FROM list_items
WHERE checked_at IS NOT NULL;

UPDATE products p
SET purchase_count = s.cnt, last_purchased_at = s.last
FROM (
    SELECT product_id, count(*)::int AS cnt, max(bought_at) AS last
    FROM purchase_events
    GROUP BY product_id
) AS s
WHERE s.product_id = p.id;

-- +goose Down
DROP INDEX list_items_checked_idx;
DROP TABLE purchase_events;
ALTER TABLE products
    DROP COLUMN stock_status,
    DROP COLUMN stock_updated_at,
    DROP COLUMN stock_updated_by,
    DROP COLUMN purchase_count,
    DROP COLUMN last_purchased_at;
