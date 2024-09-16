-- name: CreateGoal :exec
INSERT INTO goals (
    id,
    title,
    egp_subtotal,
    created_at
)
VALUES (?, ?, ?, ?);

-- name: CreateGoalItem :exec
INSERT INTO goals_items (
    goal_id,
    item_id
)
VALUES (?, ?);

-- name: GetGoals :many
SELECT
    gl.id,
    gl.title,
    gl.egp_subtotal,
    gl.created_at,
    i.id AS item_id,
    i.name AS item_name,
    i.price AS item_price,
    i.currency_id AS item_currency_id,
    i.purchased_on AS item_purchased_on,
    c.enum AS item_currency,
    c.egp_fx_rate AS item_currency_fx_rate
FROM goals AS gl
LEFT JOIN goals_items AS gi ON gl.id = gi.goal_id
LEFT JOIN items AS i ON gi.item_id = i.id
LEFT JOIN currencies AS c ON i.currency_id = c.id;

-- name: GetGoal :many
SELECT
    gl.id,
    gl.title,
    gl.egp_subtotal,
    gl.created_at,
    i.id AS item_id,
    i.name AS item_name,
    i.price AS item_price,
    i.currency_id AS item_currency_id,
    i.purchased_on AS item_purchased_on,
    c.enum AS item_currency,
    c.egp_fx_rate AS item_currency_fx_rate
FROM goals AS gl
LEFT JOIN goals_items AS gi ON gl.id = gi.goal_id
LEFT JOIN items AS i ON gi.item_id = i.id
LEFT JOIN currencies AS c ON i.currency_id = c.id
WHERE gl.id = ?;

-- name: DeleteGoal :exec
DELETE FROM goals
WHERE id = ?;
