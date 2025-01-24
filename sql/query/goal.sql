-- name: CreateGoal :exec
INSERT INTO goals (
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

-- name: GetGoals :many
SELECT
    gl.id,
    gl.name,
    gl.price,
    gl.currency_id,
    gl.fulfilled_at,
    gl.created_at,
    c.enum AS currency,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.scheduled_to AS transaction_scheduled_to,
    t.fulfilled_at AS transaction_fulfilled_at,
    a.name AS transaction_account_name,
    ac.id AS transaction_account_currency_id,
    ac.enum AS transaction_account_currency_enum
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
LEFT JOIN transactions AS t ON t.goal_id = gl.id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
ORDER BY gl.created_at DESC;


-- name: GetUnfulfilledGoals :many
SELECT
    gl.id,
    gl.name,
    gl.price,
    gl.currency_id,
    gl.fulfilled_at,
    gl.created_at,
    c.enum AS currency,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.scheduled_to AS transaction_scheduled_to,
    t.fulfilled_at AS transaction_fulfilled_at,
    a.name AS transaction_account_name,
    ac.id AS transaction_account_currency_id,
    ac.enum AS transaction_account_currency_enum
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
LEFT JOIN transactions AS t ON t.goal_id = gl.id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
WHERE gl.fulfilled_at IS NULL
ORDER BY gl.created_at DESC;


-- name: GetGoal :many
SELECT
    gl.id,
    gl.name,
    gl.price,
    gl.currency_id,
    gl.fulfilled_at,
    gl.created_at,
    c.enum AS currency,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.scheduled_to AS transaction_scheduled_to,
    t.fulfilled_at AS transaction_fulfilled_at,
    a.name AS transaction_account_name,
    ac.id AS transaction_account_currency_id,
    ac.enum AS transaction_account_currency_enum
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
LEFT JOIN transactions AS t ON t.goal_id = gl.id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
WHERE gl.id = sqlc.arg(id);

-- name: UpdateGoal :exec
UPDATE goals
SET
    name = sqlc.arg(name),
    price = sqlc.arg(price),
    currency_id = sqlc.arg(currency_id)
WHERE id = sqlc.arg(id);

-- name: FulfillGoal :exec
UPDATE goals
SET
    fulfilled_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- name: UnfulfillGoal :exec
UPDATE goals
SET
    fulfilled_at = NULL
WHERE id = sqlc.arg(id);

-- name: DeleteGoal :exec
DELETE FROM goals
WHERE id = sqlc.arg(id);
