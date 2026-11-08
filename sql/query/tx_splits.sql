-- name: GetTxSplitPeople :many
SELECT
    cp.id,
    cp.name,
    cp.icon,
    cp.color
FROM tx_split_people AS sp
JOIN counterparties AS cp ON cp.id = sp.counterparty_id
WHERE sp.transaction_id = sqlc.arg(transaction_id)
ORDER BY sp.position, cp.name;

-- name: AddTxSplitPerson :exec
INSERT INTO tx_split_people (transaction_id, counterparty_id, position)
VALUES (sqlc.arg(transaction_id), sqlc.arg(counterparty_id), sqlc.arg(position));

-- name: DeleteTxSplitPeople :exec
DELETE FROM tx_split_people
WHERE transaction_id = sqlc.arg(transaction_id);

-- The shares of the transaction's items (counterparty_id NULL = the user).
-- name: GetTxItemShares :many
SELECT s.item_id, s.counterparty_id, s.units
FROM transaction_item_shares AS s
JOIN transaction_items AS it ON it.id = s.item_id
WHERE it.transaction_id = sqlc.arg(transaction_id);

-- name: DeleteTxItemShares :exec
DELETE FROM transaction_item_shares
WHERE item_id IN (
    SELECT id FROM transaction_items WHERE transaction_id = sqlc.arg(transaction_id)
);

-- name: CreateTxItemShare :exec
INSERT INTO transaction_item_shares (item_id, counterparty_id, units)
VALUES (sqlc.arg(item_id), sqlc.narg(counterparty_id), sqlc.arg(units));

-- name: SetTxItemSharedByAll :exec
UPDATE transaction_items SET shared_by_all = sqlc.arg(shared_by_all)
WHERE id = sqlc.arg(id) AND transaction_id = sqlc.arg(transaction_id);

-- name: ClearTxItemsSharedByAll :exec
UPDATE transaction_items SET shared_by_all = FALSE
WHERE transaction_id = sqlc.arg(transaction_id);

-- Merging a counterparty into another: the bills it was in are the other's
-- (its own rows go with it, deleted).
-- name: ReassignTxSplitPeople :exec
INSERT OR IGNORE INTO tx_split_people (transaction_id, counterparty_id, position)
SELECT sp.transaction_id, sqlc.arg(to_id), sp.position FROM tx_split_people AS sp
WHERE sp.counterparty_id = sqlc.arg(from_id);

-- The units of the items both shared adds up on the one kept (the other's
-- rows are left to ReassignTxItemShares, which skips them).
-- name: MergeTxItemShareUnits :exec
UPDATE transaction_item_shares
SET units = units + (
    SELECT TOTAL(o.units) FROM transaction_item_shares AS o
    WHERE o.item_id = transaction_item_shares.item_id AND o.counterparty_id = sqlc.arg(from_id)
)
WHERE transaction_item_shares.counterparty_id = sqlc.arg(to_id)
    AND transaction_item_shares.item_id IN (
        SELECT f.item_id FROM transaction_item_shares AS f WHERE f.counterparty_id = sqlc.arg(from_id)
    );

-- name: ReassignTxItemShares :exec
UPDATE transaction_item_shares SET counterparty_id = sqlc.arg(to_id)
WHERE transaction_item_shares.counterparty_id = sqlc.arg(from_id)
    AND transaction_item_shares.item_id NOT IN (
        SELECT o.item_id FROM transaction_item_shares AS o WHERE o.counterparty_id = sqlc.arg(to_id)
    );
