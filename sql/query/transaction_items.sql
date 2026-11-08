-- name: GetTransactionItemsOf :many
SELECT * FROM transaction_items
WHERE transaction_id = sqlc.arg(transaction_id)
ORDER BY position;

-- name: GetTransactionItemsOfTxs :many
SELECT * FROM transaction_items
WHERE transaction_id IN (sqlc.slice(ids))
ORDER BY transaction_id, position;

-- name: GetRecurrentTxItemsOf :many
SELECT * FROM transaction_items
WHERE recurrent_id = sqlc.arg(recurrent_id)
ORDER BY position;

-- name: GetRecurrentTxItems :many
SELECT * FROM transaction_items
WHERE recurrent_id IS NOT NULL
ORDER BY recurrent_id, position;

-- name: CreateTransactionItem :exec
INSERT INTO transaction_items (
    id, transaction_id, recurrent_id, kind, name, quantity, unit_price, total, rate, included, shared_by_all, split_mode, position
) VALUES (
    sqlc.arg(id),
    sqlc.narg(transaction_id),
    sqlc.narg(recurrent_id),
    sqlc.arg(kind),
    sqlc.arg(name),
    sqlc.arg(quantity),
    sqlc.arg(unit_price),
    sqlc.arg(total),
    sqlc.narg(rate),
    sqlc.arg(included),
    sqlc.arg(shared_by_all),
    sqlc.arg(split_mode),
    sqlc.arg(position)
);

-- name: DeleteTransactionItems :exec
DELETE FROM transaction_items
WHERE transaction_id = sqlc.arg(transaction_id);

-- name: DeleteRecurrentTxItems :exec
DELETE FROM transaction_items
WHERE recurrent_id = sqlc.arg(recurrent_id);

-- name: DeleteTransactionItem :execrows
DELETE FROM transaction_items
WHERE id = sqlc.arg(id) AND transaction_id = sqlc.arg(transaction_id);

-- name: DeleteRecurrentTxItem :execrows
DELETE FROM transaction_items
WHERE id = sqlc.arg(id) AND recurrent_id = sqlc.arg(recurrent_id);

-- name: CountTransactionItemsOfTxs :many
SELECT transaction_id, COUNT(*) AS n FROM transaction_items
WHERE transaction_id IN (sqlc.slice(ids))
GROUP BY transaction_id;

-- The scheduled occurrences an edit of their recurrent tx applies to.
-- name: GetEditableOccurrenceIDs :many
SELECT id FROM transactions
WHERE recurrent_id = sqlc.arg(recurrent_id)
    AND fulfilled_at IS NULL
    AND NOT recurrent_overridden;

-- An occurrence edited on its own (its lines here) keeps them when its
-- recurrent tx is edited.
-- name: MarkTransactionOverridden :exec
UPDATE transactions SET recurrent_overridden = recurrent_id IS NOT NULL
WHERE id = sqlc.arg(id);
