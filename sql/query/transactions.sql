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
