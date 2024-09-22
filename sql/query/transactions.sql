-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    account_id,
    amount,
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
    t.account_id,
    t.amount,
    t.kind_id,
    t.description,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransaction :one
SELECT
    t.id,
    t.account_id,
    t.amount,
    t.kind_id,
    t.description,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
WHERE t.id = ? LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
account_id = ?,
amount = ?,
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
