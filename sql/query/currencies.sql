-- name: GetCurrency :one
SELECT
    id,
    enum,
    egp_fx_rate
FROM currencies WHERE id = ? LIMIT 1;

-- name: GetCurrencies :many
SELECT
    id,
    enum,
    egp_fx_rate
FROM currencies;

-- name: CreateCurrency :exec
INSERT INTO currencies (id, enum, egp_fx_rate)
VALUES (?, ?, ?);
