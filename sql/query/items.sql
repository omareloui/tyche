-- name: GetItems :many
SELECT
    i.id,
    i.name,
    i.price,
    i.purchased_on,
    i.currency_id,
    i.created_at,
    c.enum,
    c.egp_fx_rate
FROM items AS i
LEFT JOIN currencies AS c ON i.currency_id = c.id
ORDER BY i.created_at DESC;

-- name: GetItem :one
SELECT
    i.id,
    i.name,
    i.price,
    i.purchased_on,
    i.currency_id,
    i.created_at,
    c.enum,
    c.egp_fx_rate
FROM items AS i
LEFT JOIN currencies AS c ON i.currency_id = c.id
WHERE i.id = sqlc.arg(id) LIMIT 1;

-- name: CreateItem :exec
INSERT INTO items (
    id,
    name,
    price,
    currency_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(price),
    sqlc.arg(currency_id)
);

-- name: UpdateItem :exec
UPDATE items
SET
    name = sqlc.arg(name),
    price = sqlc.arg(price),
    currency_id = sqlc.arg(currency_id)
WHERE id = sqlc.arg(id);

-- name: DeleteItem :exec
DELETE FROM items
WHERE id = sqlc.arg(id);
