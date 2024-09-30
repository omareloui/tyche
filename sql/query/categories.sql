-- name: TotalBudget :one
SELECT TOTAL(budget_in_egp)
FROM categories
GROUP BY id;

-- name: GetCategories :many
SELECT
    c.id,
    c.name,
    c.kind_id,
    c.budget_in_egp,
    c.created_at,
    k.kind,
    k.increase_balance
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
ORDER BY c.created_at DESC;

-- name: GetCategory :one
SELECT
    c.id,
    c.name,
    c.kind_id,
    c.budget_in_egp,
    c.created_at,
    k.kind,
    k.increase_balance
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
WHERE c.id = ?;

-- name: CreateCategory :exec
INSERT INTO categories (
    id,
    name,
    kind_id,
    budget_in_egp
)
VALUES (?, ?, ?, ?);

-- name: UpdateCategory :exec
UPDATE categories
SET
name = ?,
kind_id = ?,
budget_in_egp = ?
WHERE id = ?;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = ?;
