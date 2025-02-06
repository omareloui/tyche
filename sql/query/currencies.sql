-- name: GetCurrency :one
SELECT
    sqlc.embed(currencies)
FROM currencies
WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetCurrencyByEnum :one
SELECT
    sqlc.embed(currencies)
FROM currencies
WHERE enum = sqlc.arg(enum) LIMIT 1;

-- name: GetCurrencies :many
SELECT
    sqlc.embed(currencies)
FROM currencies
ORDER BY enum;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, enum)
VALUES (sqlc.arg(id), sqlc.arg(enum));
