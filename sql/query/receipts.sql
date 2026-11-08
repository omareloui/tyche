-- name: CreateReceipt :exec
INSERT INTO receipts (id, data, content_type, hash, merchant, ocr_json, created_at)
VALUES (
    sqlc.arg(id),
    sqlc.arg(data),
    sqlc.arg(content_type),
    sqlc.arg(hash),
    sqlc.arg(merchant),
    sqlc.arg(ocr_json),
    sqlc.arg(created_at)
);

-- A transaction has one receipt: linking one drops the one it had.
-- name: UnlinkTransactionReceipts :exec
DELETE FROM receipts
WHERE transaction_id = sqlc.arg(transaction_id) AND id != sqlc.arg(keep_id);

-- name: LinkReceipt :execrows
UPDATE receipts SET transaction_id = sqlc.arg(transaction_id)
WHERE id = sqlc.arg(id) AND (transaction_id IS NULL OR transaction_id = sqlc.arg(transaction_id));

-- name: GetReceipt :one
SELECT * FROM receipts WHERE id = sqlc.arg(id);

-- name: GetReceiptOfTx :one
SELECT id, transaction_id, content_type, hash, merchant, created_at FROM receipts
WHERE transaction_id = sqlc.arg(transaction_id)
LIMIT 1;

-- name: GetReceiptsOfTxs :many
SELECT id, transaction_id, content_type, hash, CAST(LENGTH(data) AS INTEGER) AS size FROM receipts
WHERE transaction_id IN (sqlc.slice(ids));

-- name: DeleteReceipt :execrows
DELETE FROM receipts WHERE id = sqlc.arg(id);

-- name: PurgeOrphanReceipts :execrows
DELETE FROM receipts
WHERE transaction_id IS NULL AND created_at < sqlc.arg(before);

-- What was read off the same image before, preferring a receipt not linked
-- yet (reused as is) over one of a transaction (copied).
-- name: GetReceiptByHash :one
SELECT * FROM receipts
WHERE hash = sqlc.arg(hash) AND ocr_json != ''
ORDER BY transaction_id IS NULL DESC, created_at DESC
LIMIT 1;
