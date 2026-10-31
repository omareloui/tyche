-- name: ListFxRates :many
SELECT base, quote, rate FROM fx_rates;

-- name: UpsertFxRate :exec
INSERT INTO fx_rates (base, quote, rate, as_of, fetched_at)
VALUES (sqlc.arg(base), sqlc.arg(quote), sqlc.arg(rate), sqlc.arg(as_of), sqlc.arg(fetched_at))
ON CONFLICT (base, quote) DO UPDATE SET
    rate = excluded.rate,
    as_of = excluded.as_of,
    fetched_at = excluded.fetched_at;

-- name: GetFxRatesInfo :one
SELECT
    COUNT(*) AS pairs,
    COUNT(DISTINCT base) AS bases,
    CAST(COALESCE(MAX(as_of), '') AS TEXT) AS as_of,
    CAST(COALESCE(MAX(fetched_at), '') AS TEXT) AS fetched_at
FROM fx_rates;
