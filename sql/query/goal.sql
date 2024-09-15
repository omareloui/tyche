-- name: CreateGoal :exec
INSERT INTO goals (
    id,
    title,
    egp_sum_total,
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
    gl.egp_sum_total,
    gl.created_at,
    i.bought_on AS item_bought_on,
    i.id AS item_id,
    i.name AS item_name,
    i.amount AS item_amount,
    i.currency_id AS item_currency_id,
    c.enum AS item_currency,
    c.egp_fx_rate AS item_currency_fx_rate
FROM goals AS gl
LEFT JOIN goals_items AS gi ON gl.id = gi.goal_id
LEFT JOIN items AS i ON gi.item_id = i.id
LEFT JOIN currencies AS c ON i.currency_id = c.id;

-- name: GetGoal :one
SELECT
    gl.id,
    gl.title,
    gl.egp_sum_total,
    gl.created_at,
    i.bought_on AS item_bought_on,
    i.id AS item_id,
    i.name AS item_name,
    i.amount AS item_amount,
    i.currency_id AS item_currency_id,
    c.enum AS item_currency,
    c.egp_fx_rate AS item_currency_fx_rate
FROM goals AS gl
LEFT JOIN goals_items AS gi ON gl.id = gi.goal_id
LEFT JOIN items AS i ON gi.item_id = i.id
LEFT JOIN currencies AS c ON i.currency_id = c.id
WHERE gl.id = ? LIMIT 1;
