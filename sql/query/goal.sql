-- name: CreateGoal :exec
INSERT INTO goals (
    id,
    name,
    price,
    currency_id,
    account_id
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(price),
    sqlc.arg(currency_id),
    sqlc.arg(account_id)
);

-- name: GetGoals :many
SELECT
    sqlc.embed(gl),
    sqlc.embed(c),
    sqlc.embed(ga),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    t.loan_goal_id AS transaction_loan_goal_id,
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
    ac.symbol AS account_currency_symbol,
    ac.in_grams AS account_currency_in_grams
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
JOIN accounts AS ga ON ga.id = gl.account_id
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id -- deposits, withdrawals, purchase, loan payments
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
ORDER BY gl.created_at DESC;


-- name: GetUnfulfilledGoals :many
SELECT
    sqlc.embed(gl),
    sqlc.embed(c),
    sqlc.embed(ga),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    t.loan_goal_id AS transaction_loan_goal_id,
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
    ac.symbol AS account_currency_symbol,
    ac.in_grams AS account_currency_in_grams
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
JOIN accounts AS ga ON ga.id = gl.account_id
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id -- deposits, withdrawals, purchase, loan payments
LEFT JOIN accounts AS a ON a.id = t.account_id
LEFT JOIN currencies AS ac ON ac.id = a.currency_id
WHERE gl.fulfilled_at IS NULL
ORDER BY gl.created_at DESC;


-- name: GetGoal :many
SELECT
    sqlc.embed(gl),
    sqlc.embed(c),
    sqlc.embed(ga),
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind AS transaction_kind,
    t.category_id AS transaction_category_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    t.loan_goal_id AS transaction_loan_goal_id,
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
    ac.symbol AS account_currency_symbol,
    ac.in_grams AS account_currency_in_grams
FROM goals AS gl
LEFT JOIN currencies AS c ON c.id = gl.currency_id
JOIN accounts AS ga ON ga.id = gl.account_id
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id -- deposits, withdrawals, purchase, loan payments
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

-- name: SetGoalAccount :exec
UPDATE goals
SET account_id = sqlc.arg(account_id)
WHERE id = sqlc.arg(id);

-- name: GetGoalsWithoutAccount :many
SELECT * FROM goals
WHERE account_id IS NULL;

-- name: CountAccountTransactions :one
SELECT COUNT(*) FROM transactions
WHERE account_id = sqlc.arg(id) OR to_account_id = sqlc.arg(id);

-- name: GetGoalPurchaseID :one
-- The expense out of the goal's account recording its purchase.
SELECT t.id FROM transactions AS t
JOIN goals AS gl ON gl.account_id = t.account_id
WHERE gl.id = sqlc.arg(id) AND t.kind = 1
ORDER BY t.created_at DESC
LIMIT 1;

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

-- name: GetGoalContributionsSince :many
-- Money put towards each goal since a date (deposits net of withdrawals), in
-- the goal's currency.
SELECT
    gl.id AS goal_id,
    CAST(TOTAL(IIF(t.to_account_id = gl.account_id, COALESCE(t.to_amount, t.amount), -t.amount)) AS FLOAT) AS amount
FROM transactions AS t
JOIN goals AS gl ON gl.account_id IN (t.account_id, t.to_account_id)
WHERE
    t.kind = 3
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(since)
GROUP BY gl.id;

-- name: UpdateGoalAccount :exec
-- Keeps a goal's account in sync with it.
UPDATE accounts
SET
    name = sqlc.arg(name),
    currency_id = sqlc.arg(currency_id)
WHERE id = sqlc.arg(id);

-- name: GetAccountKindID :one
SELECT id FROM account_kinds
WHERE enum = sqlc.arg(enum);

-- name: SetGoalPlan :exec
UPDATE goals
SET plan = sqlc.arg(plan)
WHERE id = sqlc.arg(id);

-- name: LinkLoanPayments :exec
UPDATE transactions
SET loan_goal_id = sqlc.arg(goal_id)
WHERE id IN (SELECT value FROM json_each(sqlc.arg(ids)));

-- name: GetUnpaidLoanPayments :many
SELECT id FROM transactions
WHERE loan_goal_id = sqlc.arg(goal_id) AND fulfilled_at IS NULL;

-- name: UnlinkLoanPayments :exec
UPDATE transactions
SET loan_goal_id = NULL
WHERE loan_goal_id = sqlc.arg(goal_id);
