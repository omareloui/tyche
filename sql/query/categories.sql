-- name: GetCategories :many
SELECT
    sqlc.embed(c),
    cur.id AS budget_currency_id,
    cur.string AS budget_currency_string,
    cur.symbol AS budget_currency_symbol,
    sqlc.embed(k)
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
LEFT JOIN currencies AS cur ON cur.id = c.budget_currency_id
ORDER BY c.created_at;

-- name: GetCategory :one
SELECT
    sqlc.embed(c),
    cur.id AS budget_currency_id,
    cur.string AS budget_currency_string,
    cur.symbol AS budget_currency_symbol,
    sqlc.embed(k)
FROM categories AS c
LEFT JOIN transaction_kinds AS k ON c.kind_id = k.id
LEFT JOIN currencies AS cur ON cur.id = c.budget_currency_id
WHERE c.id = sqlc.arg(id);

-- name: CreateCategory :exec
INSERT INTO categories (
    id,
    name,
    kind_id,
    budget_amount,
    budget_currency_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(kind_id),
    sqlc.arg(budget_amount),
    sqlc.arg(budget_currency_id)
);

-- name: UpdateCategory :exec
UPDATE categories
SET
    name = sqlc.arg(name),
    kind_id = sqlc.arg(kind_id),
    budget_amount = sqlc.arg(budget_amount),
    budget_currency_id = sqlc.arg(budget_currency_id)
WHERE id = sqlc.arg(id);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = sqlc.arg(id);
