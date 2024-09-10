-- name: GetBalances :many
SELECT
    balances.amount,
    balances.currency_id,
    currencies.currency
FROM balances
LEFT JOIN currencies
    ON balances.currency_id = currencies.id;

-- name: CreateBalance :exec
INSERT INTO balances (id, currency_id, amount) VALUES (?, ?, ?);
