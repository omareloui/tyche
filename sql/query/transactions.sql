-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    amount,
    currency_id,
    kind_id,
    fulfilled_on,
    scheduled_to
)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetTransactions :many
SELECT
    t.id,
    t.amount,
    t.currency_id,
    t.kind_id,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    c.currency,
    k.kind
FROM transactions AS t
LEFT JOIN currencies AS c ON c.id = transactions.currency_id
LEFT JOIN transaction_kinds AS k ON k.id = transactions.kind_id;
