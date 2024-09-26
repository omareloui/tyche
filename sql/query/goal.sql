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
    c.egp_fx_rate AS item_currency_fx_rate,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.scheduled_to AS transaction_scheduled_to,
    t.fulfilled_on AS transaction_fulfilled_on,
    a.name AS transaction_account_name,
    ac.id AS transaction_account_currency_id,
    ac.enum AS transaction_account_currency_enum,
    ac.egp_fx_rate AS transaction_account_currency_fx_rate
FROM goals AS gl
LEFT JOIN goals_items AS gi ON gl.id = gi.goal_id
LEFT JOIN items AS i ON gi.item_id = i.id
LEFT JOIN currencies AS c ON i.currency_id = c.id
LEFT JOIN goals_progresses AS gp ON gp.goal_id = gl.id
LEFT JOIN transactions AS t ON t.id = gp.transaction_id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id;

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

-- name: UpdateGoal :exec
UPDATE goals
SET 
title = ?,
egp_subtotal = ?
WHERE id = ?;

-- name: DeleteGoalItems :exec
DELETE FROM goals_items
WHERE goal_id = ?;

-- name: DeleteGoal :exec
DELETE FROM goals
WHERE id = ?;
