-- name: GetItems :many
SELECT
    sqlc.embed(items),
    cat.name AS category_name,
    cat.color AS category_color,
    (SELECT COUNT(*) FROM counterparty_items AS ci WHERE ci.item_id = items.id) AS merchants,
    img.hash AS image_hash,
    img.width AS image_width,
    img.height AS image_height
FROM items
LEFT JOIN item_categories AS cat ON items.category_id = cat.id
LEFT JOIN item_images AS img ON img.item_id = items.id
ORDER BY items.name COLLATE NOCASE;

-- name: GetItem :one
SELECT
    sqlc.embed(items),
    cat.name AS category_name,
    cat.color AS category_color,
    (SELECT COUNT(*) FROM counterparty_items AS ci WHERE ci.item_id = items.id) AS merchants,
    img.hash AS image_hash,
    img.width AS image_width,
    img.height AS image_height
FROM items
LEFT JOIN item_categories AS cat ON items.category_id = cat.id
LEFT JOIN item_images AS img ON img.item_id = items.id
WHERE items.id = sqlc.arg(id);

-- name: GetItemIDs :many
-- The ids that are items, of ids.
SELECT id FROM items WHERE id IN (sqlc.slice(ids));

-- name: GetItemCandidates :many
-- Every item, to match a line's name to (models.MatchItem); at_merchant
-- tells it was bought at counterparty_id before.
SELECT
    id, name_key, aliases, created_at,
    CAST(EXISTS (
        SELECT 1 FROM counterparty_items AS ci
        WHERE ci.item_id = items.id AND ci.counterparty_id = sqlc.narg(counterparty_id)
    ) AS BOOLEAN) AS at_merchant
FROM items;

-- name: CreateItem :exec
INSERT INTO items (id, name, name_key, aliases, category_id)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(name_key), sqlc.arg(aliases), sqlc.narg(category_id));

-- name: UpdateItem :exec
UPDATE items
SET
    name = sqlc.arg(name),
    name_key = sqlc.arg(name_key),
    aliases = sqlc.arg(aliases),
    category_id = sqlc.narg(category_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteItem :exec
DELETE FROM items WHERE id = sqlc.arg(id);

-- name: GetItemImage :one
SELECT data, content_type, hash FROM item_images WHERE item_id = sqlc.arg(item_id);

-- name: SetItemImage :exec
INSERT INTO item_images (item_id, data, content_type, hash, width, height)
VALUES (sqlc.arg(item_id), sqlc.arg(data), sqlc.arg(content_type), sqlc.arg(hash), sqlc.arg(width), sqlc.arg(height))
ON CONFLICT (item_id) DO UPDATE SET
    data = excluded.data,
    content_type = excluded.content_type,
    hash = excluded.hash,
    width = excluded.width,
    height = excluded.height,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteItemImage :exec
DELETE FROM item_images WHERE item_id = sqlc.arg(item_id);

-- name: MoveItemImage :exec
-- Merging items: from_id's picture becomes into_id's when it has none.
UPDATE OR IGNORE item_images SET item_id = sqlc.arg(into_id) WHERE item_id = sqlc.arg(from_id);

-- name: CountItemLines :one
SELECT COUNT(*) FROM transaction_items WHERE item_id = sqlc.arg(item_id);

-- name: MoveItemLines :exec
-- Merging items: the lines of from_id become into_id's.
UPDATE transaction_items SET item_id = sqlc.arg(into_id) WHERE item_id = sqlc.arg(from_id);

-- name: MoveItemMerchants :exec
-- Merging items: from_id's merchants become into_id's (the ones it already
-- has go with from_id's delete).
UPDATE OR IGNORE counterparty_items SET item_id = sqlc.arg(into_id) WHERE item_id = sqlc.arg(from_id);

-- name: MoveCounterpartyItems :exec
-- Merging counterparties: from_id's items become into_id's (the ones it
-- already has go with from_id's delete).
UPDATE OR IGNORE counterparty_items SET counterparty_id = sqlc.arg(into_id) WHERE counterparty_id = sqlc.arg(from_id);

-- name: LinkCounterpartyItemsOfTx :exec
-- The transaction's merchant sells its lines' items.
INSERT OR IGNORE INTO counterparty_items (counterparty_id, item_id)
SELECT t.counterparty_id, ti.item_id
FROM transaction_items AS ti
JOIN transactions AS t ON t.id = ti.transaction_id
WHERE t.id = sqlc.arg(transaction_id)
    AND t.counterparty_id IS NOT NULL
    AND ti.item_id IS NOT NULL;

-- name: LinkCounterpartyItemsOfRecurrent :exec
INSERT OR IGNORE INTO counterparty_items (counterparty_id, item_id)
SELECT r.counterparty_id, ti.item_id
FROM transaction_items AS ti
JOIN recurrent_txs AS r ON r.id = ti.recurrent_id
WHERE r.id = sqlc.arg(recurrent_id)
    AND r.counterparty_id IS NOT NULL
    AND ti.item_id IS NOT NULL;

-- name: GetItemLines :many
-- The item lines of fulfilled expenses (net of nothing: lines are what was
-- bought, never money moved), of one item or every one (item_id NULL), dated
-- (scheduled_to) from from_date to to_date (exclusive, either NULL for no
-- bound), oldest
-- first, with what models.ItemsRate needs to bring them to their account's
-- currency: lines_total (TxItemsTotal) and the lines' own currency.
SELECT
    ti.item_id,
    ti.quantity,
    ti.total,
    ti.currency_id AS lines_currency_id,
    ti.warranty_until,
    t.warranty_until AS tx_warranty_until,
    t.id AS transaction_id,
    t.scheduled_to,
    t.amount,
    CAST((
        SELECT TOTAL(lt.total) FROM transaction_items AS lt
        WHERE lt.transaction_id = t.id AND (lt.kind = 0 OR NOT lt.included)
    ) AS FLOAT) AS lines_total,
    sqlc.embed(cur),
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color
FROM transaction_items AS ti
JOIN transactions AS t ON t.id = ti.transaction_id
JOIN accounts AS a ON a.id = t.account_id
JOIN currencies AS cur ON cur.id = a.currency_id
LEFT JOIN counterparties AS cp ON cp.id = t.counterparty_id
WHERE ti.kind = 0
    AND ti.item_id IS NOT NULL
    AND (sqlc.narg(item_id) IS NULL OR ti.item_id = sqlc.narg(item_id))
    AND t.kind = 1
    AND t.fulfilled_at IS NOT NULL
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND NOT t.for_others
    AND (sqlc.narg(from_date) IS NULL OR t.scheduled_to >= sqlc.narg(from_date))
    AND (sqlc.narg(to_date) IS NULL OR t.scheduled_to < sqlc.narg(to_date))
ORDER BY t.scheduled_to, t.id, ti.position;

-- name: GetChargeLines :many
-- The charge lines (VAT, service, tip, ...) of fulfilled expenses, like
-- GetItemLines, of one counterparty or every one (counterparty_id NULL),
-- dated (scheduled_to) from from_date to to_date (exclusive, either NULL for
-- no bound), oldest first, with what models.ItemsRate needs and the items'
-- subtotal its rate is of.
SELECT
    ti.kind,
    ti.total,
    ti.rate,
    ti.included,
    ti.currency_id AS lines_currency_id,
    t.id AS transaction_id,
    t.scheduled_to,
    t.amount,
    CAST((
        SELECT TOTAL(lt.total) FROM transaction_items AS lt
        WHERE lt.transaction_id = t.id AND (lt.kind = 0 OR NOT lt.included)
    ) AS FLOAT) AS lines_total,
    CAST((
        SELECT TOTAL(st.total) FROM transaction_items AS st
        WHERE st.transaction_id = t.id AND st.kind = 0
    ) AS FLOAT) AS subtotal,
    sqlc.embed(cur),
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color
FROM transaction_items AS ti
JOIN transactions AS t ON t.id = ti.transaction_id
JOIN accounts AS a ON a.id = t.account_id
JOIN currencies AS cur ON cur.id = a.currency_id
LEFT JOIN counterparties AS cp ON cp.id = t.counterparty_id
WHERE ti.kind > 0
    AND (sqlc.narg(counterparty_id) IS NULL OR t.counterparty_id = sqlc.narg(counterparty_id))
    AND t.kind = 1
    AND t.fulfilled_at IS NOT NULL
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND NOT t.for_others
    AND (sqlc.narg(from_date) IS NULL OR t.scheduled_to >= sqlc.narg(from_date))
    AND (sqlc.narg(to_date) IS NULL OR t.scheduled_to < sqlc.narg(to_date))
ORDER BY t.scheduled_to, t.id, ti.position;

-- name: SearchItems :many
-- The items whose name or an alias holds q (lowercased), the ones bought at
-- counterparty_id first, then the most bought, with the last line's unit
-- price and currency (at that merchant when bought there).
SELECT
    items.id,
    items.name,
    CAST(EXISTS (
        SELECT 1 FROM counterparty_items AS ci
        WHERE ci.item_id = items.id AND ci.counterparty_id = sqlc.narg(counterparty_id)
    ) AS BOOLEAN) AS at_merchant,
    (SELECT COUNT(*) FROM transaction_items AS c WHERE c.item_id = items.id) AS times,
    CAST(COALESCE((
        SELECT IIF(lt.quantity > 0, lt.total / lt.quantity, lt.unit_price)
        FROM transaction_items AS lt
        JOIN transactions AS t ON t.id = lt.transaction_id
        WHERE lt.item_id = items.id
            AND (t.counterparty_id = sqlc.narg(counterparty_id) OR NOT EXISTS (
                SELECT 1 FROM transaction_items AS ml
                JOIN transactions AS mt ON mt.id = ml.transaction_id
                WHERE ml.item_id = items.id AND mt.counterparty_id = sqlc.narg(counterparty_id)
            ))
        ORDER BY t.scheduled_to DESC
        LIMIT 1
    ), 0) AS FLOAT) AS last_price,
    CAST(COALESCE((
        SELECT lc.string
        FROM transaction_items AS lt
        JOIN transactions AS t ON t.id = lt.transaction_id
        JOIN accounts AS a ON a.id = t.account_id
        JOIN currencies AS lc ON lc.id = COALESCE(lt.currency_id, a.currency_id)
        WHERE lt.item_id = items.id
            AND (t.counterparty_id = sqlc.narg(counterparty_id) OR NOT EXISTS (
                SELECT 1 FROM transaction_items AS ml
                JOIN transactions AS mt ON mt.id = ml.transaction_id
                WHERE ml.item_id = items.id AND mt.counterparty_id = sqlc.narg(counterparty_id)
            ))
        ORDER BY t.scheduled_to DESC
        LIMIT 1
    ), '') AS TEXT) AS last_currency
FROM items
WHERE INSTR(LOWER(items.name), sqlc.arg(q)) > 0 OR INSTR(LOWER(items.aliases), sqlc.arg(q)) > 0
ORDER BY at_merchant DESC, times DESC, items.name COLLATE NOCASE
LIMIT sqlc.arg(lim);

-- name: GetTransactionCounterpartyID :one
SELECT counterparty_id FROM transactions WHERE id = sqlc.arg(id);

-- name: GetRecurrentTxCounterpartyID :one
SELECT counterparty_id FROM recurrent_txs WHERE id = sqlc.arg(id);

-- name: GetItemCategories :many
-- Every item category, by name, with how many items are in it.
SELECT
    sqlc.embed(item_categories),
    (SELECT COUNT(*) FROM items AS i WHERE i.category_id = item_categories.id) AS items
FROM item_categories
ORDER BY item_categories.name COLLATE NOCASE;

-- name: GetItemCategory :one
SELECT * FROM item_categories WHERE id = sqlc.arg(id);

-- name: GetItemCategoryByName :one
SELECT * FROM item_categories WHERE name = sqlc.arg(name) COLLATE NOCASE;

-- name: CreateItemCategory :exec
INSERT INTO item_categories (id, name, color) VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(color));

-- name: UpdateItemCategory :exec
UPDATE item_categories SET name = sqlc.arg(name), color = sqlc.arg(color) WHERE id = sqlc.arg(id);

-- name: DeleteItemCategory :exec
-- Its items keep going, without a category (ON DELETE SET NULL).
DELETE FROM item_categories WHERE id = sqlc.arg(id);

-- name: SetItemCategory :exec
UPDATE items
SET category_id = sqlc.narg(category_id), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);
