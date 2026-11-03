-- name: GetCurrency :one
SELECT
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default
FROM currencies AS c
LEFT JOIN user AS u ON c.id = u.default_currency_id
WHERE c.id = sqlc.arg(id)
LIMIT 1;

-- name: GetDefaultCurrency :one
SELECT
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default
FROM currencies AS c
LEFT JOIN user AS u ON c.id = u.default_currency_id
WHERE u.id IS NOT NULL
LIMIT 1;


-- name: GetCurrencyByString :one
SELECT
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default
FROM currencies AS c
LEFT JOIN user AS u ON c.id = u.default_currency_id
WHERE c.string = sqlc.arg(string)
LIMIT 1;

-- name: GetCurrencies :many
SELECT
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default
FROM currencies AS c
LEFT JOIN user AS u ON c.id = u.default_currency_id
WHERE c.is_shown = TRUE
ORDER BY is_default DESC, c.id;


-- name: GetAllCurrencies :many
SELECT
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default
FROM currencies AS c
LEFT JOIN user AS u ON c.id = u.default_currency_id
ORDER BY is_default DESC, is_shown DESC, c.id;


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

-- name: SetCurrencyInGrams :exec
UPDATE currencies
SET
    in_grams = sqlc.arg(in_grams)
WHERE id = sqlc.arg(id);
