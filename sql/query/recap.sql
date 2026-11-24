-- name: GetFirstSeenCounterparties :many
-- The merchants whose first fulfilled transaction is between from_date and
-- to_date (exclusive), with what was spent at them then per account currency,
-- net like GetMerchantSpending, for the month recap's new merchants.
SELECT
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    sqlc.embed(cur),
    CAST(TOTAL(CASE
        WHEN t.kind = 1 THEN t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        )
        WHEN t.kind IN (2, 5) THEN -t.amount
        ELSE 0
    END) AS FLOAT) AS total
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    cp.is_merchant IS TRUE
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND NOT t.for_others
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date)
    AND NOT EXISTS (
        SELECT 1 FROM transactions AS o
        WHERE
            o.counterparty_id = cp.id
            AND o.fulfilled_at IS NOT NULL
            AND o.scheduled_to < sqlc.arg(from_date)
    )
GROUP BY cp.id, cur.id;

-- name: GetRecapExpenses :many
-- The fulfilled expenses between from_date and to_date (exclusive) with what
-- got reimbursed of them, transfer pair legs left out, for the month recap's
-- largest transactions.
SELECT
    t.id,
    t.amount,
    t.description,
    t.scheduled_to,
    t.transfer_pair_id,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed,
    a.id AS account_id,
    a.name AS account_name,
    sqlc.embed(cur),
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
WHERE
    t.kind = 1
    AND t.transfer_pair_id IS NULL
    AND NOT t.for_others
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date);

-- name: GetGoalContributionsBetween :many
-- GetGoalContributionsSince bounded by to_date (exclusive), for the month
-- recap.
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
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date)
    AND NOT EXISTS (
        SELECT 1 FROM accounts AS o
        WHERE o.goal_id = ga.goal_id AND o.id != ga.id AND o.id IN (t.account_id, t.to_account_id)
    )
GROUP BY ga.goal_id, gc.string;
