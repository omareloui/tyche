-- name: CreateTransaction :exec
INSERT INTO transactions (
    id,
    account_id,
    amount,
    kind,
    category_id,
    counterparty_id,
    description,
    to_account_id,
    to_amount,
    goal_id,
    recurrent_id,
    scheduled_to,
    fulfilled_at,
    needs_review
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 2),
    sqlc.arg(kind),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    sqlc.arg(goal_id),
    sqlc.arg(recurrent_id),
    sqlc.arg(scheduled_to),
    sqlc.arg(fulfilled_at),
    sqlc.arg(needs_review)
);

-- name: GetTransactions :many
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    CAST(EXISTS (
        SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id
    ) AS BOOLEAN) AS from_sms
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON t.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransaction :one
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    CAST(EXISTS (
        SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id
    ) AS BOOLEAN) AS from_sms
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN goals AS g ON t.goal_id = g.id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE t.id = sqlc.arg(id) LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 2),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.narg(to_account_id),
    to_amount = ROUND(sqlc.narg(to_amount), 2),
    scheduled_to = sqlc.arg(scheduled_to),
    goal_id = sqlc.arg(goal_id),
    needs_review = FALSE
WHERE id = sqlc.arg(id);

-- name: FulfillTransaction :exec
UPDATE transactions
SET
    fulfilled_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = sqlc.arg(id);

-- name: MarkTransactionReviewed :exec
UPDATE transactions
SET
    needs_review = FALSE
WHERE id = sqlc.arg(id);

-- name: GetMerchantSpending :many
-- Sums fulfilled expenses (kind 1) per merchant and account currency for a
-- YYYY-MM month; currencies are merged in Go using live fx rates.
SELECT
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    sqlc.embed(cur),
    TOTAL(t.amount) AS total
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind = 1
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS TRUE
    AND STRFTIME('%Y-%m', t.fulfilled_at) = CAST(sqlc.arg(month) AS TEXT)
GROUP BY cp.id, cur.id
ORDER BY total DESC;

-- name: GetMerchantSpendingMonths :many
SELECT DISTINCT CAST(STRFTIME('%Y-%m', t.fulfilled_at) AS TEXT) AS month
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
WHERE
    t.kind = 1
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS TRUE
ORDER BY month DESC;
