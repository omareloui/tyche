-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    amount,
    currency_id,
    kind_id,
    is_recurrent,
    scheduled_to,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetTransactions :many
SELECT
    t.id,
    t.amount,
    t.currency_id,
    t.kind_id,
    t.is_recurrent,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    c.currency,
    c.symbol,
    k.kind
FROM transactions AS t
LEFT JOIN currencies AS c ON t.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id;
