-- name: GetCurrency :one
SELECT
    id,
    enum
FROM currencies
WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetCurrencies :many
SELECT
    id,
    enum
FROM currencies
ORDER BY enum;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, enum)
VALUES (sqlc.arg(id), sqlc.arg(enum));
