-- name: GetCurrency :one
SELECT
    id,
    currency
FROM currencies WHERE id = ? LIMIT 1;

-- name: GetCurrencies :many
SELECT
    id,
    currency
FROM currencies;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, currency)
VALUES (?, ?);
