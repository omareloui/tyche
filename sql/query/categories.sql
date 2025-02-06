-- name: TotalBudget :one
SELECT TOTAL(budget_in_egp)
FROM categories
GROUP BY id;

-- name: GetCategories :many
SELECT
    sqlc.embed(c),
    sqlc.embed(k)
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
ORDER BY c.created_at;

-- name: GetCategory :one
SELECT
    sqlc.embed(c),
    sqlc.embed(k)
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
WHERE c.id = sqlc.arg(id);

-- name: CreateCategory :exec
INSERT INTO categories (
    id,
    name,
    kind_id,
    budget_in_egp
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(kind_id),
    sqlc.arg(budget_in_egp)
);

-- name: UpdateCategory :exec
UPDATE categories
SET
    name = sqlc.arg(name),
    kind_id = sqlc.arg(kind_id),
    budget_in_egp = sqlc.arg(budget_in_egp)
WHERE id = sqlc.arg(id);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = sqlc.arg(id);
