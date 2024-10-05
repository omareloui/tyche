-- name: GetCurrency :one
SELECT
    id,
    enum,
    egp_fx_rate
FROM currencies WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetCurrencies :many
SELECT
    id,
    enum,
    egp_fx_rate
FROM currencies
ORDER BY enum;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, enum, egp_fx_rate)
VALUES (sqlc.arg(id), sqlc.arg(enum), sqlc.arg(egp_fx_rate));
