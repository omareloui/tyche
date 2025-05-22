-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    account_id,
    amount,
    kind_id,
    category_id,
    counterparty_id,
    description,
    to_account_id,
    to_amount,
    goal_id,
    scheduled_to,
    fulfilled_at
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 2),
    sqlc.arg(kind_id),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    sqlc.arg(goal_id),
    sqlc.arg(scheduled_to),
    sqlc.arg(fulfilled_at)
);

-- name: GetTransactions :many
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    cp.name AS counterparty_name,
    sqlc.embed(ta),
    sqlc.embed(tc),
    sqlc.embed(k),
    sqlc.embed(cat),
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.enum AS goal_currency_enum
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON t.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransaction :one
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    cp.name AS counterparty_name,
    sqlc.embed(ta),
    sqlc.embed(tc),
    sqlc.embed(k),
    sqlc.embed(cat),
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.enum AS goal_currency_enum
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON t.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE t.id = sqlc.arg(id) LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 2),
    kind_id = sqlc.arg(kind_id),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.narg(to_account_id),
    to_amount = ROUND(sqlc.narg(to_amount), 2),
    scheduled_to = sqlc.arg(scheduled_to),
    goal_id = sqlc.arg(goal_id)
WHERE id = sqlc.arg(id);

-- name: FulfillTransaction :exec
UPDATE transactions
SET
    fulfilled_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = sqlc.arg(id);
