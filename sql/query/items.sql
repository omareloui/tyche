-- name: GetItems :many
SELECT
    i.id,
    i.name,
    i.amount,
    i.bought_on,
    i.currency_id,
    c.enum,
    c.egp_fx_rate
FROM items AS i
LEFT JOIN currencies AS c ON i.currency_id = c.id
ORDER BY i.amount;

-- name: GetItem :one
SELECT
    i.id,
    i.name,
    i.amount,
    i.bought_on,
    i.currency_id,
    c.enum,
    c.egp_fx_rate
FROM items AS i
LEFT JOIN currencies AS c ON i.currency_id = c.id
WHERE i.id = ? LIMIT 1;

-- name: CreateItem :exec
INSERT INTO items (
    id,
    name,
    amount,
    currency_id
)
VALUES (?, ?, ?, ?);

-- name: UpdateItem :exec
UPDATE items
SET
name = ?,
amount = ?,
currency_id = ?
WHERE id = ?;

-- name: DeleteItem :exec
DELETE FROM items
WHERE id = ?;
