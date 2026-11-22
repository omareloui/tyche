-- name: ListColPPP :many
SELECT country, series, year, value FROM col_ppp;

-- name: UpsertColPPP :exec
INSERT INTO col_ppp (country, series, year, value)
VALUES (sqlc.arg(country), sqlc.arg(series), sqlc.arg(year), sqlc.arg(value))
ON CONFLICT (country, series, year) DO UPDATE SET value = excluded.value;

-- name: ListColInflation :many
SELECT country, year, pct FROM col_inflation;

-- name: UpsertColInflation :exec
INSERT INTO col_inflation (country, year, pct)
VALUES (sqlc.arg(country), sqlc.arg(year), sqlc.arg(pct))
ON CONFLICT (country, year) DO UPDATE SET pct = excluded.pct;

-- name: GetColCityEstimate :one
SELECT estimate, model, created_at FROM col_city_estimates WHERE key = sqlc.arg(key);

-- name: SetColCityEstimate :exec
INSERT INTO col_city_estimates (key, estimate, model, created_at)
VALUES (sqlc.arg(key), sqlc.arg(estimate), sqlc.arg(model), sqlc.arg(created_at))
ON CONFLICT (key) DO UPDATE SET estimate = excluded.estimate, model = excluded.model, created_at = excluded.created_at;
