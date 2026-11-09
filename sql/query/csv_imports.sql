-- name: CreateCSVImport :exec
INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count)
VALUES (sqlc.arg(id), sqlc.arg(account_id), sqlc.arg(file_name), sqlc.arg(content), sqlc.arg(mapping), sqlc.arg(row_count));

-- name: GetCSVImport :one
SELECT * FROM csv_imports WHERE id = sqlc.arg(id);

-- name: ListCSVImports :many
-- The latest imports, the account's name for the list.
SELECT
    i.id,
    i.account_id,
    i.file_name,
    i.status,
    i.row_count,
    i.imported_count,
    i.created_at,
    i.imported_at,
    a.name AS account_name
FROM csv_imports AS i
JOIN accounts AS a ON i.account_id = a.id
WHERE i.status = 'imported'
ORDER BY i.imported_at DESC
LIMIT sqlc.arg(lim);

-- name: UpdateCSVImportMapping :exec
UPDATE csv_imports SET mapping = sqlc.arg(mapping) WHERE id = sqlc.arg(id);

-- name: MarkCSVImportImported :exec
UPDATE csv_imports
SET status = 'imported',
    content = NULL,
    mapping = sqlc.arg(mapping),
    imported_count = sqlc.arg(imported_count),
    imported_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: PurgePendingCSVImports :execrows
DELETE FROM csv_imports WHERE status = 'pending' AND created_at < sqlc.arg(before);

-- name: SetTransactionImport :exec
UPDATE transactions
SET import_id = sqlc.arg(import_id), import_hash = sqlc.arg(import_hash)
WHERE id = sqlc.arg(id);

-- name: GetCSVMapping :one
SELECT mapping FROM csv_mappings
WHERE account_id = sqlc.arg(account_id) AND header_sig = sqlc.arg(header_sig);

-- name: SaveCSVMapping :exec
INSERT INTO csv_mappings (id, account_id, header_sig, mapping)
VALUES (sqlc.arg(id), sqlc.arg(account_id), sqlc.arg(header_sig), sqlc.arg(mapping))
ON CONFLICT (account_id, header_sig) DO UPDATE
SET mapping = excluded.mapping, updated_at = CURRENT_TIMESTAMP;

-- name: GetCSVDupCandidates :many
-- The fulfilled transactions moving account_id between since and until, as
-- what a CSV row may already be: its side (1 out, 2 in) and the amount it
-- moved the account by.
SELECT
    t.id,
    t.fulfilled_at,
    t.scheduled_to,
    CAST(COALESCE(t.import_hash, '') AS TEXT) AS import_hash,
    CAST(CASE
        WHEN t.to_account_id = sqlc.arg(account_id) THEN 2
        WHEN t.kind IN (2, 5) THEN 2 -- income, refund
        WHEN t.kind = 6 AND t.amount > 0 THEN 2 -- adjustment
        ELSE 1
    END AS INTEGER) AS side,
    CAST(ABS(IIF(t.to_account_id = sqlc.arg(account_id), COALESCE(t.to_amount, t.amount), t.amount)) AS FLOAT) AS moved
FROM transactions AS t
WHERE (t.account_id = sqlc.arg(account_id) OR t.to_account_id = sqlc.arg(account_id))
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(since)
    AND t.scheduled_to < sqlc.arg(until)
ORDER BY t.scheduled_to;
