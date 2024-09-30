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
ORDER BY i.price;

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
WHERE i.id = ? LIMIT 1;

-- name: CreateItem :exec
INSERT INTO items (
    id,
    name,
    price,
    currency_id
)
VALUES (?, ?, ?, ?);

-- name: UpdateItem :exec
UPDATE items
SET
name = ?,
price = ?,
currency_id = ?
WHERE id = ?;

-- name: DeleteItem :exec
DELETE FROM items
WHERE id = ?;
