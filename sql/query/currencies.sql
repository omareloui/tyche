-- name: GetCurrency :one
SELECT
    id,
    currency,
    symbol
FROM currencies WHERE id = ? LIMIT 1;

-- name: GetCurrencies :many
SELECT
    id,
    currency,
    symbol
FROM currencies;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, currency, symbol)
VALUES (?, ?, ?);
