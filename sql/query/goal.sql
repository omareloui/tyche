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
-- deposits, withdrawals, conversions, purchase (in or out of its accounts),
-- loan payments
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id OR EXISTS (
    SELECT 1 FROM accounts AS gh WHERE gh.goal_id = gl.id AND gh.id IN (t.account_id, t.to_account_id)
)
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
-- deposits, withdrawals, conversions, purchase (in or out of its accounts),
-- loan payments
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id OR EXISTS (
    SELECT 1 FROM accounts AS gh WHERE gh.goal_id = gl.id AND gh.id IN (t.account_id, t.to_account_id)
)
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
-- deposits, withdrawals, conversions, purchase (in or out of its accounts),
-- loan payments
LEFT JOIN transactions AS t ON gl.account_id IN (t.account_id, t.to_account_id) OR t.loan_goal_id = gl.id OR EXISTS (
    SELECT 1 FROM accounts AS gh WHERE gh.goal_id = gl.id AND gh.id IN (t.account_id, t.to_account_id)
)
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
SELECT gl.id, gl.name, gl.currency_id, c.string AS currency
FROM goals AS gl
JOIN currencies AS c ON c.id = gl.currency_id
WHERE gl.account_id IS NULL;

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
-- Money put towards each goal since a date (deposits net of withdrawals), per
-- currency of its accounts. Conversions between its accounts are left out.
SELECT
    ga.goal_id AS goal_id,
    gc.string AS currency,
    CAST(TOTAL(IIF(t.to_account_id = ga.id, COALESCE(t.to_amount, t.amount), -t.amount)) AS FLOAT) AS amount
FROM transactions AS t
JOIN accounts AS ga ON ga.goal_id IS NOT NULL AND ga.id IN (t.account_id, t.to_account_id)
JOIN currencies AS gc ON gc.id = ga.currency_id
WHERE
    t.kind = 3
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(since)
    AND NOT EXISTS (
        SELECT 1 FROM accounts AS o
        WHERE o.goal_id = ga.goal_id AND o.id != ga.id AND o.id IN (t.account_id, t.to_account_id)
    )
GROUP BY ga.goal_id, gc.string;

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

-- name: SetAccountGoal :exec
-- Ties an account to the goal it holds money of.
UPDATE accounts
SET goal_id = sqlc.arg(goal_id)
WHERE id = sqlc.arg(id);

-- name: GetGoalHoldings :many
-- The live accounts of goals besides their own: the money put in from other
-- currencies, one account per currency.
SELECT
    sqlc.embed(a),
    sqlc.embed(c)
FROM accounts AS a
JOIN currencies AS c ON c.id = a.currency_id
WHERE
    a.goal_id IS NOT NULL
    AND a.deleted_at IS NULL
    AND (sqlc.narg(goal_id) IS NULL OR a.goal_id = sqlc.narg(goal_id))
    AND NOT EXISTS (SELECT 1 FROM goals AS gl WHERE gl.account_id = a.id)
ORDER BY a.created_at;

-- name: GetGoalConversionIDs :many
-- The transfers between the goal's accounts: its holdings converted to its
-- currency when buying it.
SELECT t.id FROM transactions AS t
JOIN accounts AS a ON a.id = t.account_id
JOIN accounts AS ta ON ta.id = t.to_account_id
WHERE a.goal_id = sqlc.arg(goal_id) AND ta.goal_id = sqlc.arg(goal_id);

-- name: GetGoalsToSplit :many
-- The goals whose own account isn't tied to them yet (made before goals held
-- money per currency), with its currency.
SELECT gl.id, gl.name, gl.account_id, gl.fulfilled_at, a.currency_id, c.string AS currency
FROM goals AS gl
JOIN accounts AS a ON a.id = gl.account_id
JOIN currencies AS c ON c.id = a.currency_id
WHERE a.goal_id IS NULL;

-- name: GetGoalAccountTransfers :many
-- The fulfilled or scheduled transfers in or out of an account with the
-- other side's currency, oldest first.
SELECT
    t.id,
    t.account_id,
    t.to_account_id,
    t.amount,
    t.to_amount,
    t.fulfilled_at,
    oc.id AS other_currency_id,
    oc.string AS other_currency
FROM transactions AS t
JOIN accounts AS o ON o.id = IIF(t.account_id = sqlc.arg(id), t.to_account_id, t.account_id)
JOIN currencies AS oc ON oc.id = o.currency_id
WHERE t.kind = 3 AND (t.account_id = sqlc.arg(id) OR t.to_account_id = sqlc.arg(id))
ORDER BY t.scheduled_to, t.created_at;

-- name: MoveTransferTo :exec
-- Points a transfer's to side at another account.
UPDATE transactions
SET to_account_id = sqlc.arg(to_account_id), to_amount = sqlc.arg(to_amount)
WHERE id = sqlc.arg(id);

-- name: MoveTransferFrom :exec
-- Points a transfer's from side at another account.
UPDATE transactions
SET account_id = sqlc.arg(account_id), amount = sqlc.arg(amount)
WHERE id = sqlc.arg(id);
