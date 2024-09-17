-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    amount,
    currency_id,
    kind_id,
    description,
    scheduled_to,
    fulfilled_on,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTransactions :many
SELECT
    t.id,
    t.amount,
    t.currency_id,
    t.kind_id,
    t.description,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    c.enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance
FROM transactions AS t
LEFT JOIN currencies AS c ON t.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransaction :one
SELECT
    t.id,
    t.amount,
    t.currency_id,
    t.kind_id,
    t.description,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    c.enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance
FROM transactions AS t
LEFT JOIN currencies AS c ON t.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
WHERE t.id = ? LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
amount = ?,
currency_id = ?,
kind_id = ?,
description = ?,
scheduled_to = ?
WHERE id = ?;

-- name: FulfillTransaction :exec
UPDATE transactions
SET
fulfilled_on = ?
WHERE id = ?;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;

----------------

-- name: CreateFrequentTransaction :exec
INSERT INTO frequent_transactions (
    id,
    frequency_by,
    frequency,
    amount,
    currency_id,
    kind_id,
    description,
    stops_at,
    created_at,
    last_transaction_created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);


-- name: CreateFrequentTransactionTransaction :exec
INSERT INTO frequent_transactions_transactions (
    transaction_id,
    frequency_id
)
VALUES (?, ?);
