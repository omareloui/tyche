-- name: GetCurrency :one
SELECT
    sqlc.embed(currencies)
FROM currencies
WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetCurrencyByString :one
SELECT
    sqlc.embed(currencies)
FROM currencies
WHERE string = sqlc.arg(string) LIMIT 1;

-- name: GetCurrencies :many
SELECT
    sqlc.embed(currencies)
FROM currencies;

-- name: CreateCurrency :exec
INSERT INTO currencies (
    id,
    string,
    symbol,
    kind,
    iban_prefix
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(string),
    sqlc.arg(symbol),
    sqlc.arg(kind),
    sqlc.arg(iban_prefix)
);
