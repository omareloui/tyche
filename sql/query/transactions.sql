-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    account_id,
    amount,
    kind_id,
    category_id,
    description,
    to_account_id,
    to_amount,
    scheduled_to,
    fulfilled_on
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTransactions :many
SELECT
    t.id,
    t.account_id,
    t.amount,
    t.kind_id,
    t.category_id,
    t.description,
    t.to_account_id,
    t.to_amount,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    ta.name AS to_account_name,
    ta.currency_id AS to_currency_id,
    tc.enum AS to_currency_enum,
    tc.egp_fx_rate AS to_egp_fx_rate,
    k.kind,
    k.increase_balance,
    cat.name AS category_name,
    cat.budget_in_egp AS category_budget_in_egp,
    g.title AS goal_title,
    g.egp_subtotal AS goal_egp_subtotal,
    IFNULL(t.amount * c.egp_fx_rate / g.egp_subtotal, 0.0) AS goal_percentage
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
LEFT JOIN goals_progresses AS gp ON t.id = gp.transaction_id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON gp.goal_id = g.id
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransaction :one
SELECT
    t.id,
    t.account_id,
    t.amount,
    t.kind_id,
    t.category_id,
    t.description,
    t.to_account_id,
    t.to_amount,
    t.fulfilled_on,
    t.scheduled_to,
    t.created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    ta.name AS to_account_name,
    ta.currency_id AS to_currency_id,
    tc.enum AS to_currency_enum,
    tc.egp_fx_rate AS to_egp_fx_rate,
    k.kind,
    k.increase_balance,
    cat.name AS category_name,
    cat.budget_in_egp AS category_budget_in_egp,
    g.title AS goal_title,
    g.egp_subtotal AS goal_egp_subtotal,
    IFNULL(t.amount * c.egp_fx_rate / g.egp_subtotal, 0.0) AS goal_percentage
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN transaction_kinds AS k ON t.kind_id = k.id
LEFT JOIN goals_progresses AS gp ON t.id = gp.transaction_id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON gp.goal_id = g.id
WHERE t.id = ? LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
account_id = ?,
amount = ?,
kind_id = ?,
category_id = ?,
description = ?,
to_account_id = ?,
to_amount = ?,
scheduled_to = ?
WHERE id = ?;

-- name: FulfillTransaction :exec
UPDATE transactions
SET
fulfilled_on = DATETIME('now')
WHERE id = ?;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = ?;
