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
    sqlc.embed(gl),
    sqlc.embed(c),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.goal_id AS transaction_goal_id,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    a.id AS account_id,
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,
    ac.id AS account_currency_id,
    ac.string AS account_currency_string,
    ac.symbol AS account_currency_symbol
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
LEFT JOIN transactions AS t ON t.goal_id = gl.id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
ORDER BY gl.created_at DESC;


-- name: GetUnfulfilledGoals :many
SELECT
    sqlc.embed(gl),
    sqlc.embed(c),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.goal_id AS transaction_goal_id,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    a.id AS account_id,
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,
    ac.id AS account_currency_id,
    ac.string AS account_currency_string,
    ac.symbol AS account_currency_symbol
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
LEFT JOIN transactions AS t ON t.goal_id = gl.id
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
WHERE gl.fulfilled_at IS NULL
ORDER BY gl.created_at DESC;


-- name: GetGoal :many
SELECT
    sqlc.embed(gl),
    sqlc.embed(c),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.goal_id AS transaction_goal_id,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    a.id AS account_id,
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,
    ac.id AS account_currency_id,
    ac.string AS account_currency_string,
    ac.symbol AS account_currency_symbol
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
