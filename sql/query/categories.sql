-- name: GetCategories :many
SELECT
    sqlc.embed(c),
    cur.id AS budget_currency_id,
    cur.string AS budget_currency_string,
    cur.symbol AS budget_currency_symbol
FROM categories AS c
LEFT JOIN currencies AS cur ON cur.id = c.budget_currency_id
ORDER BY c.created_at;

-- name: GetCategory :one
SELECT
    sqlc.embed(c),
    cur.id AS budget_currency_id,
    cur.string AS budget_currency_string,
    cur.symbol AS budget_currency_symbol
FROM categories AS c
LEFT JOIN currencies AS cur ON cur.id = c.budget_currency_id
WHERE c.id = sqlc.arg(id);

-- name: CreateCategory :exec
INSERT INTO categories (
    id,
    name,
    color,
    kinds,
    budget_amount,
    budget_currency_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(color),
    sqlc.arg(kinds),
    sqlc.arg(budget_amount),
    sqlc.arg(budget_currency_id)
);

-- name: UpdateCategory :exec
UPDATE categories
SET
    name = sqlc.arg(name),
    color = sqlc.arg(color),
    kinds = sqlc.arg(kinds),
    budget_amount = sqlc.arg(budget_amount),
    budget_currency_id = sqlc.arg(budget_currency_id)
WHERE id = sqlc.arg(id);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = sqlc.arg(id);
