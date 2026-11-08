-- name: AddReceiptScan :exec
INSERT INTO receipt_scans (id, tenant_id, scanned_at, model, input_tokens, output_tokens, cost_micros, ok, error, cached)
VALUES (
    sqlc.arg(id),
    sqlc.arg(tenant_id),
    sqlc.arg(scanned_at),
    sqlc.arg(model),
    sqlc.arg(input_tokens),
    sqlc.arg(output_tokens),
    sqlc.arg(cost_micros),
    sqlc.arg(ok),
    sqlc.arg(error),
    sqlc.arg(cached)
);

-- The tenant's billed scans in [from, to): the ones that used tokens (never
-- the cached ones).
-- name: CountReceiptScans :one
SELECT COUNT(*) FROM receipt_scans
WHERE tenant_id = sqlc.arg(tenant_id)
    AND scanned_at >= sqlc.arg(from_at) AND scanned_at < sqlc.arg(to_at)
    AND NOT cached
    AND (ok OR input_tokens > 0 OR output_tokens > 0);

-- Every scan since, oldest first, aggregated by the admin page.
-- name: ListReceiptScansSince :many
SELECT * FROM receipt_scans
WHERE scanned_at >= sqlc.arg(from_at)
ORDER BY scanned_at;

-- name: GetReceiptLimit :one
SELECT monthly_limit FROM receipt_limits WHERE tenant_id = sqlc.arg(tenant_id);

-- name: ListReceiptLimits :many
SELECT * FROM receipt_limits;

-- name: SetReceiptLimit :exec
INSERT INTO receipt_limits (tenant_id, monthly_limit, updated_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(monthly_limit), sqlc.arg(updated_at))
ON CONFLICT (tenant_id) DO UPDATE SET
    monthly_limit = excluded.monthly_limit,
    updated_at = excluded.updated_at;

-- name: DeleteReceiptLimit :exec
DELETE FROM receipt_limits WHERE tenant_id = sqlc.arg(tenant_id);
