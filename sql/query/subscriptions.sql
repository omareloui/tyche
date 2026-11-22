-- name: GetSubscriptionTxs :many
-- The fulfilled expenses with a counterparty not made by a recurrent
-- transaction since from_date, for models.DetectSubscriptions: net of their
-- reimbursements, following the analysis rule (no reimbursements, transfer
-- pair legs, goals' or loan payments), out of live accounts, oldest first.
SELECT
    t.id,
    t.account_id,
    a.currency_id,
    CAST(t.counterparty_id AS TEXT) AS counterparty_id,
    CAST(IFNULL(t.category_id, '') AS TEXT) AS category_id,
    CAST(SUBSTR(t.scheduled_to, 1, 10) AS TEXT) AS day,
    CAST(t.amount - (
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS amount
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
WHERE
    t.kind = 1
    AND t.counterparty_id IS NOT NULL
    AND t.recurrent_id IS NULL
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND t.loan_goal_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND a.goal_id IS NULL
    AND a.deleted_at IS NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
ORDER BY t.scheduled_to, t.id;

-- name: GetDismissedSubscriptions :many
SELECT counterparty_id, currency_id, amount FROM dismissed_subscriptions;

-- name: DismissSubscription :exec
INSERT INTO dismissed_subscriptions (counterparty_id, currency_id, amount)
VALUES (?, ?, ?)
ON CONFLICT (counterparty_id, currency_id) DO UPDATE SET
    amount = excluded.amount,
    dismissed_at = CURRENT_TIMESTAMP;
