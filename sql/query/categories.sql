-- name: TotalBudget :one
SELECT TOTAL(budget_in_egp)
FROM categories
GROUP BY id;

-- name: GetCategories :many
SELECT
    id,
    name,
    budget_in_egp,
    created_at
FROM categories
ORDER BY created_at DESC;

-- name: GetCategory :one
SELECT
    id,
    name,
    budget_in_egp,
    created_at
FROM categories
WHERE id = ?;

-- name: CreateCategory :exec
INSERT INTO categories (
    id,
    name,
    budget_in_egp
)
VALUES (?, ?, ?);

-- name: UpdateCategory :exec
UPDATE categories
SET
name = ?,
budget_in_egp = ?
WHERE id = ?;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = ?;
