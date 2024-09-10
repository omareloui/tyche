-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    amount,
    currency_id,
    transaction_kind_id,
    can_repeat,
    fulfilled_on,
    scheduled_to
)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetTransactions :many
SELECT
    t.id,
    t.amount,
    t.currency_id,
    t.transaction_kind_id,
    t.can_repeat,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    c.currency,
    k.kind
FROM transactions AS t
LEFT JOIN currencies AS c ON c.id = transactions.currency_id
LEFT JOIN transaction_kinds AS k ON k.id = transactions.transaction_kind_id;
