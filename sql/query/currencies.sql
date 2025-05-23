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
FROM currencies
WHERE currencies.is_shown = TRUE;


-- name: GetAllCurrencies :many
SELECT
    sqlc.embed(currencies)
FROM currencies;


-- name: CreateCurrency :exec
INSERT INTO currencies (
    id,
    string,
    symbol,
    kind,
    iban_prefix,
    is_shown
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(string),
    sqlc.arg(symbol),
    sqlc.arg(kind),
    sqlc.arg(iban_prefix),
    sqlc.arg(is_shown)
);


-- name: UpdateCurrency :exec
UPDATE currencies
SET
    string = sqlc.arg(string),
    symbol = sqlc.arg(symbol),
    kind = sqlc.arg(kind),
    iban_prefix = sqlc.arg(iban_prefix)
WHERE id = sqlc.arg(id);

-- name: SetCurrencyShown :exec
UPDATE currencies
SET
    is_shown = sqlc.arg(is_shown)
WHERE id = sqlc.arg(id);
