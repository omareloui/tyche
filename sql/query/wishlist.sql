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
    installment_plan,
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
    sqlc.arg(installment_plan),
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
    installment_plan = sqlc.arg(installment_plan),
    url = sqlc.arg(url),
    note = sqlc.arg(note),
    cooling_days = sqlc.arg(cooling_days)
WHERE id = sqlc.arg(id);

-- name: GetWishlistItemByGoal :one
SELECT
    sqlc.embed(w),
    sqlc.embed(c)
FROM wishlist_items AS w
LEFT JOIN currencies AS c ON c.id = w.currency_id
WHERE w.goal_id = sqlc.arg(goal_id);

-- Back to waiting, it's no longer saved for.
-- name: SetWishlistItemStatus :exec
UPDATE wishlist_items
SET
    status = sqlc.arg(status),
    transaction_id = sqlc.arg(transaction_id),
    decided_at = CASE WHEN sqlc.arg(status) = 0 THEN NULL ELSE DATETIME('now') END,
    goal_id = CASE WHEN sqlc.arg(status) = 0 THEN NULL ELSE goal_id END
WHERE id = sqlc.arg(id);

-- name: LinkWishlistItemToGoal :exec
UPDATE wishlist_items
SET
    status = 3, -- models.WishlistSaving
    goal_id = sqlc.arg(goal_id),
    transaction_id = NULL,
    decided_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- Decides the wish the goal saves for, if any, as the goal is bought (with
-- the transaction), bought undone or deleted.
-- name: SetWishlistItemStatusByGoal :exec
UPDATE wishlist_items
SET
    status = sqlc.arg(status),
    transaction_id = sqlc.arg(transaction_id),
    decided_at = DATETIME('now')
WHERE goal_id = sqlc.arg(goal_id);

-- name: DeleteWishlistItem :exec
DELETE FROM wishlist_items
WHERE id = sqlc.arg(id);
