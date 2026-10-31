-- name: GetWishlistItems :many
SELECT
    sqlc.embed(w),
    sqlc.embed(c)
FROM wishlist_items AS w
LEFT JOIN currencies AS c ON c.id = w.currency_id
ORDER BY w.created_at DESC;

-- name: GetWishlistItem :one
SELECT
    sqlc.embed(w),
    sqlc.embed(c)
FROM wishlist_items AS w
LEFT JOIN currencies AS c ON c.id = w.currency_id
WHERE w.id = sqlc.arg(id);

-- name: CreateWishlistItem :exec
INSERT INTO wishlist_items (
    id,
    name,
    price,
    currency_id,
    installments,
    installment_amount,
    url,
    note,
    cooling_days,
    created_at
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(price),
    sqlc.arg(currency_id),
    sqlc.arg(installments),
    sqlc.arg(installment_amount),
    sqlc.arg(url),
    sqlc.arg(note),
    sqlc.arg(cooling_days),
    sqlc.arg(created_at)
);

-- name: UpdateWishlistItem :exec
UPDATE wishlist_items
SET
    name = sqlc.arg(name),
    price = sqlc.arg(price),
    currency_id = sqlc.arg(currency_id),
    installments = sqlc.arg(installments),
    installment_amount = sqlc.arg(installment_amount),
    url = sqlc.arg(url),
    note = sqlc.arg(note),
    cooling_days = sqlc.arg(cooling_days)
WHERE id = sqlc.arg(id);

-- name: SetWishlistItemStatus :exec
UPDATE wishlist_items
SET
    status = sqlc.arg(status),
    transaction_id = sqlc.arg(transaction_id),
    decided_at = CASE WHEN sqlc.arg(status) = 0 THEN NULL ELSE DATETIME('now') END
WHERE id = sqlc.arg(id);

-- name: DeleteWishlistItem :exec
DELETE FROM wishlist_items
WHERE id = sqlc.arg(id);
