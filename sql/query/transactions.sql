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
    recurrent_id,
    scheduled_to,
    fulfilled_at,
    needs_review
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 6),
    sqlc.arg(kind),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 6), NULL),
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
    ak.enum AS account_kind_enum,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    tak.enum AS to_account_kind_enum,
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
    ta.deleted_at AS to_account_deleted_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.id AS goal_id,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    gc.in_grams AS goal_currency_in_grams,
    CAST(COALESCE((
        SELECT sm.body FROM sms_messages AS sm WHERE sm.transaction_id = t.id LIMIT 1
    ), '') AS TEXT) AS sms_body,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN account_kinds AS ak ON a.kind_id = ak.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN account_kinds AS tak ON ta.kind_id = tak.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
-- A goal's transactions are the ones in or out of its accounts, and the
-- payments of the loan it was bought with.
LEFT JOIN goals AS g ON g.id IN (a.goal_id, ta.goal_id) OR g.id = t.loan_goal_id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
-- Top-level transactions only: reimbursements come nested under their
-- expense (GetReimbursementsOfMany) and the incoming leg of a transfer pair
-- under its outgoing one (GetTransferPairsOfMany). Every filter is optional
-- (NULL = any); a transfer pair counts as a transfer.
WHERE
    t.reimburses_id IS NULL
    AND NOT (t.transfer_pair_id IS NOT NULL AND t.kind = 2)
    AND (sqlc.narg(q) IS NULL
        OR INSTR(LOWER(t.description), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cp.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cp.aliases), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cat.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(a.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(ta.name), sqlc.narg(q)) > 0
        OR EXISTS (
            SELECT 1 FROM transactions AS rt
            LEFT JOIN counterparties AS rcp ON rt.counterparty_id = rcp.id
            WHERE rt.reimburses_id = t.id
                AND (INSTR(LOWER(rt.description), sqlc.narg(q)) > 0 OR INSTR(LOWER(rcp.name), sqlc.narg(q)) > 0)
        )
        OR EXISTS (
            SELECT 1 FROM transactions AS pt
            JOIN accounts AS pa ON pt.account_id = pa.id
            WHERE pt.id = t.transfer_pair_id
                AND (INSTR(LOWER(pt.description), sqlc.narg(q)) > 0 OR INSTR(LOWER(pa.name), sqlc.narg(q)) > 0)
        ))
    AND (sqlc.narg(kind) IS NULL
        OR (t.transfer_pair_id IS NULL AND t.kind = sqlc.narg(kind))
        OR (t.transfer_pair_id IS NOT NULL AND sqlc.narg(kind) = 3))
    AND (sqlc.narg(account_id) IS NULL OR t.account_id = sqlc.narg(account_id) OR t.to_account_id = sqlc.narg(account_id)
        OR EXISTS (
            SELECT 1 FROM transactions AS pt WHERE pt.id = t.transfer_pair_id AND pt.account_id = sqlc.narg(account_id)
        ))
    AND (sqlc.narg(category_id) IS NULL OR t.category_id = sqlc.narg(category_id)
        OR (sqlc.narg(category_id) = 'none' AND t.category_id IS NULL))
    AND (sqlc.narg(counterparty_id) IS NULL OR t.counterparty_id = sqlc.narg(counterparty_id)
        OR (sqlc.narg(counterparty_id) = 'none' AND t.counterparty_id IS NULL))
    AND (sqlc.narg(goal_id) IS NULL OR t.loan_goal_id = sqlc.narg(goal_id) OR EXISTS (
        SELECT 1 FROM accounts AS fa WHERE fa.goal_id = sqlc.narg(goal_id) AND fa.id IN (t.account_id, t.to_account_id)
    ))
    AND (sqlc.narg(fulfilled) IS NULL OR (t.fulfilled_at IS NOT NULL) = sqlc.narg(fulfilled))
    AND (sqlc.narg(needs_review) IS NULL OR (t.needs_review OR EXISTS (
        SELECT 1 FROM transactions AS rt WHERE rt.reimburses_id = t.id AND rt.needs_review
    ) OR EXISTS (
        SELECT 1 FROM transactions AS pt WHERE pt.id = t.transfer_pair_id AND pt.needs_review
    )) = sqlc.narg(needs_review))
    AND (sqlc.narg(from_sms) IS NULL OR EXISTS (
        SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id
    ) = sqlc.narg(from_sms))
    AND (sqlc.narg(has_receipt) IS NULL OR EXISTS (
        SELECT 1 FROM receipts AS rc WHERE rc.transaction_id = t.id
    ) = sqlc.narg(has_receipt))
    AND (sqlc.narg(from_date) IS NULL OR t.scheduled_to >= sqlc.narg(from_date))
    AND (sqlc.narg(to_date) IS NULL OR t.scheduled_to < sqlc.narg(to_date))
    AND (sqlc.narg(min_amount) IS NULL OR t.amount >= sqlc.narg(min_amount))
    AND (sqlc.narg(max_amount) IS NULL OR t.amount <= sqlc.narg(max_amount))
    -- Not a filter of the list, for the details' other occurrences.
    AND (sqlc.narg(recurrent_tx_id) IS NULL OR t.recurrent_id = sqlc.narg(recurrent_tx_id))
    AND (sqlc.narg(before) IS NULL OR t.scheduled_to < sqlc.narg(before))
    AND (sqlc.narg(after) IS NULL OR t.scheduled_to >= sqlc.narg(after))
-- By day (scheduled_to's date, as the list groups them), then by the time
-- shown (fulfilled_at, scheduled_to for the scheduled ones): a day picked in
-- a form is stored at midnight UTC, and julianday compares the times stored
-- in different zones.
ORDER BY SUBSTR(t.scheduled_to, 1, 10) DESC,
    JULIANDAY(COALESCE(t.fulfilled_at, t.scheduled_to)) DESC,
    t.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetTransactionsSummary :many
-- What the transactions list's filter matches as a whole, per account
-- currency and fulfilled or not: how many rows the list shows, what they
-- earn and spend following the analysis rule (expenses net of their
-- reimbursements, refunds netted against spending, transfer pairs left out).
-- Keep the WHERE in sync with GetTransactions'.
SELECT
    sqlc.embed(c),
    CAST(t.fulfilled_at IS NOT NULL AS BOOLEAN) AS fulfilled,
    COUNT(*) AS count,
    CAST(TOTAL(IIF(t.kind = 2 AND t.transfer_pair_id IS NULL, t.amount, 0)) AS FLOAT) AS income,
    CAST(TOTAL(CASE WHEN t.transfer_pair_id IS NOT NULL THEN 0 ELSE CASE t.kind
        WHEN 1 THEN t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        )
        WHEN 5 THEN -t.amount
        ELSE 0
    END END) AS FLOAT) AS expenses
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
WHERE
    t.reimburses_id IS NULL
    AND NOT (t.transfer_pair_id IS NOT NULL AND t.kind = 2)
    AND (sqlc.narg(q) IS NULL
        OR INSTR(LOWER(t.description), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cp.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cp.aliases), sqlc.narg(q)) > 0
        OR INSTR(LOWER(cat.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(a.name), sqlc.narg(q)) > 0
        OR INSTR(LOWER(ta.name), sqlc.narg(q)) > 0
        OR EXISTS (
            SELECT 1 FROM transactions AS rt
            LEFT JOIN counterparties AS rcp ON rt.counterparty_id = rcp.id
            WHERE rt.reimburses_id = t.id
                AND (INSTR(LOWER(rt.description), sqlc.narg(q)) > 0 OR INSTR(LOWER(rcp.name), sqlc.narg(q)) > 0)
        )
        OR EXISTS (
            SELECT 1 FROM transactions AS pt
            JOIN accounts AS pa ON pt.account_id = pa.id
            WHERE pt.id = t.transfer_pair_id
                AND (INSTR(LOWER(pt.description), sqlc.narg(q)) > 0 OR INSTR(LOWER(pa.name), sqlc.narg(q)) > 0)
        ))
    AND (sqlc.narg(kind) IS NULL
        OR (t.transfer_pair_id IS NULL AND t.kind = sqlc.narg(kind))
        OR (t.transfer_pair_id IS NOT NULL AND sqlc.narg(kind) = 3))
    AND (sqlc.narg(account_id) IS NULL OR t.account_id = sqlc.narg(account_id) OR t.to_account_id = sqlc.narg(account_id)
        OR EXISTS (
            SELECT 1 FROM transactions AS pt WHERE pt.id = t.transfer_pair_id AND pt.account_id = sqlc.narg(account_id)
        ))
    AND (sqlc.narg(category_id) IS NULL OR t.category_id = sqlc.narg(category_id)
        OR (sqlc.narg(category_id) = 'none' AND t.category_id IS NULL))
    AND (sqlc.narg(counterparty_id) IS NULL OR t.counterparty_id = sqlc.narg(counterparty_id)
        OR (sqlc.narg(counterparty_id) = 'none' AND t.counterparty_id IS NULL))
    AND (sqlc.narg(goal_id) IS NULL OR t.loan_goal_id = sqlc.narg(goal_id) OR EXISTS (
        SELECT 1 FROM accounts AS fa WHERE fa.goal_id = sqlc.narg(goal_id) AND fa.id IN (t.account_id, t.to_account_id)
    ))
    AND (sqlc.narg(fulfilled) IS NULL OR (t.fulfilled_at IS NOT NULL) = sqlc.narg(fulfilled))
    AND (sqlc.narg(needs_review) IS NULL OR (t.needs_review OR EXISTS (
        SELECT 1 FROM transactions AS rt WHERE rt.reimburses_id = t.id AND rt.needs_review
    ) OR EXISTS (
        SELECT 1 FROM transactions AS pt WHERE pt.id = t.transfer_pair_id AND pt.needs_review
    )) = sqlc.narg(needs_review))
    AND (sqlc.narg(from_sms) IS NULL OR EXISTS (
        SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id
    ) = sqlc.narg(from_sms))
    AND (sqlc.narg(has_receipt) IS NULL OR EXISTS (
        SELECT 1 FROM receipts AS rc WHERE rc.transaction_id = t.id
    ) = sqlc.narg(has_receipt))
    AND (sqlc.narg(from_date) IS NULL OR t.scheduled_to >= sqlc.narg(from_date))
    AND (sqlc.narg(to_date) IS NULL OR t.scheduled_to < sqlc.narg(to_date))
    AND (sqlc.narg(min_amount) IS NULL OR t.amount >= sqlc.narg(min_amount))
    AND (sqlc.narg(max_amount) IS NULL OR t.amount <= sqlc.narg(max_amount))
GROUP BY c.id, fulfilled;

-- name: CountTransactionsNeedingReview :one
SELECT COUNT(*) FROM transactions WHERE needs_review;

-- name: CountOverdueTransactions :one
-- The scheduled transactions due before today, counted like the
-- transactions list's upcoming ones (top-level only).
SELECT COUNT(*) FROM transactions AS t
WHERE t.fulfilled_at IS NULL
    AND t.scheduled_to < sqlc.arg(today)
    AND t.reimburses_id IS NULL
    AND NOT (t.transfer_pair_id IS NOT NULL AND t.kind = 2);

-- name: ListBadgeTransactions :many
-- The transactions to review (needs_review) or overdue (the ones
-- CountOverdueTransactions counts), for the badge's notifications.
SELECT
    t.id,
    t.kind,
    t.amount,
    t.description,
    t.scheduled_to,
    t.needs_review,
    a.name AS account_name,
    c.symbol AS currency_symbol,
    c.in_grams AS currency_in_grams,
    cp.name AS counterparty_name,
    cat.name AS category_name
FROM transactions AS t
JOIN accounts AS a ON a.id = t.account_id
JOIN currencies AS c ON c.id = a.currency_id
LEFT JOIN counterparties AS cp ON cp.id = t.counterparty_id
LEFT JOIN categories AS cat ON cat.id = t.category_id
WHERE CASE WHEN CAST(sqlc.arg(review) AS BOOLEAN) THEN t.needs_review ELSE
    t.fulfilled_at IS NULL
    AND t.scheduled_to < sqlc.arg(today)
    AND t.reimburses_id IS NULL
    AND NOT (t.transfer_pair_id IS NOT NULL AND t.kind = 2)
END
ORDER BY t.scheduled_to DESC, t.id DESC
LIMIT sqlc.arg(max);

-- name: GetTransaction :one
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    ak.enum AS account_kind_enum,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    tak.enum AS to_account_kind_enum,
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
    ta.deleted_at AS to_account_deleted_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.id AS goal_id,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    gc.in_grams AS goal_currency_in_grams,
    CAST(COALESCE((
        SELECT sm.body FROM sms_messages AS sm WHERE sm.transaction_id = t.id LIMIT 1
    ), '') AS TEXT) AS sms_body,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN account_kinds AS ak ON a.kind_id = ak.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN account_kinds AS tak ON ta.kind_id = tak.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
-- A goal's transactions are the ones in or out of its accounts, and the
-- payments of the loan it was bought with.
LEFT JOIN goals AS g ON g.id IN (a.goal_id, ta.goal_id) OR g.id = t.loan_goal_id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE t.id = sqlc.arg(id) LIMIT 1;

-- name: UpdateTransaction :exec
UPDATE transactions
SET
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 6),
    kind = sqlc.arg(kind),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.narg(to_account_id),
    to_amount = ROUND(sqlc.narg(to_amount), 6),
    scheduled_to = sqlc.arg(scheduled_to),
    needs_review = FALSE,
    -- An occurrence edited on its own keeps its edit when its recurrent
    -- transaction is edited.
    recurrent_overridden = recurrent_id IS NOT NULL
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

-- name: MarkTransactionNeedsReview :exec
UPDATE transactions
SET
    needs_review = TRUE
WHERE id = sqlc.arg(id);

-- name: MarkAllTransactionsReviewed :exec
UPDATE transactions
SET
    needs_review = FALSE
WHERE needs_review = TRUE;

-- name: ReviewTransactionCategory :exec
UPDATE transactions
SET
    category_id = sqlc.narg(category_id),
    needs_review = FALSE
WHERE id = sqlc.arg(id);

-- name: GetMerchantSpending :many
-- Sums fulfilled expenses (kind 1, net of their reimbursements) net of refunds
-- (kind 5) and incomes (kind 2, treated as refunds from merchants) per merchant and
-- account currency fulfilled between from_date and to_date (exclusive);
-- currencies are merged in Go using live fx rates.
SELECT
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    sqlc.embed(cur),
    TOTAL(IIF(
        t.kind = 1,
        t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        ),
        -t.amount
    )) AS total
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 2, 5)
    -- Reimbursements are netted from their expense.
    AND t.reimburses_id IS NULL
    -- Transfer pairs move money between own accounts.
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS TRUE
    AND t.fulfilled_at >= sqlc.arg(from_date)
    AND t.fulfilled_at < sqlc.arg(to_date)
GROUP BY cp.id, cur.id
ORDER BY total DESC;

-- name: GetMerchantSpendingDays :many
-- The days (YYYY-MM-DD) with merchant expenses, latest first, for the months
-- to navigate to.
SELECT DISTINCT CAST(SUBSTR(t.fulfilled_at, 1, 10) AS TEXT) AS day
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
WHERE
    t.kind = 1
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS TRUE
ORDER BY day DESC;

-- name: GetMerchantDailySpending :many
-- GetMerchantSpending per day (fulfilled_at's) too, with the merchants'
-- labels, for the counterparties summary.
SELECT
    CAST(SUBSTR(t.fulfilled_at, 1, 10) AS TEXT) AS day,
    cp.id AS counterparty_id,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    cp.labels AS counterparty_labels,
    sqlc.embed(cur),
    CAST(TOTAL(IIF(
        t.kind = 1,
        t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        ),
        -t.amount
    )) AS FLOAT) AS total
FROM transactions AS t
JOIN counterparties AS cp ON t.counterparty_id = cp.id
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 2, 5)
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS TRUE
    AND t.fulfilled_at >= sqlc.arg(from_date)
    AND t.fulfilled_at < sqlc.arg(to_date)
GROUP BY day, cp.id, cur.id
ORDER BY day;

-- name: SetTransactionReimburses :exec
UPDATE transactions
SET
    reimburses_id = sqlc.narg(reimburses_id)
WHERE id = sqlc.arg(id);

-- name: ClearReimbursementsOf :exec
UPDATE transactions
SET
    reimburses_id = NULL
WHERE reimburses_id = sqlc.arg(id);

-- name: GetReimbursedTotal :one
SELECT CAST(TOTAL(amount) AS FLOAT) AS total
FROM transactions
WHERE reimburses_id = sqlc.arg(id) AND id != sqlc.arg(exclude_id);

-- name: GetReimbursableExpenses :many
-- Expenses in the given currency scheduled in [since, until), newest first,
-- that still have at least min_left left to be reimbursed, not out of a goal's
-- account (its purchase) nor a deleted one.
SELECT
    t.id,
    t.amount,
    t.description,
    t.scheduled_to,
    a.name AS account_name,
    cur.symbol AS currency_symbol,
    cur.in_grams AS currency_in_grams,
    cp.name AS counterparty_name,
    cat.name AS category_name,
    cat.color AS category_color,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
WHERE
    t.kind = 1
    AND a.kind_id NOT IN (SELECT id FROM account_kinds WHERE enum = 6) -- models.AccKindGoal
    AND a.deleted_at IS NULL
    AND t.transfer_pair_id IS NULL
    AND a.currency_id = sqlc.arg(currency_id)
    AND t.scheduled_to >= sqlc.arg(since)
    AND t.scheduled_to < sqlc.arg(until)
    AND t.amount - (
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) >= CAST(sqlc.arg(min_left) AS FLOAT)
ORDER BY t.scheduled_to DESC, t.created_at DESC
LIMIT 50;

-- name: GetReimbursementsOf :many
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    cat.name AS category_name
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
WHERE t.reimburses_id = sqlc.arg(id)
ORDER BY t.scheduled_to, t.created_at;

-- name: GetReimbursementsOfMany :many
-- The reimbursements of the expenses ids (a JSON array), oldest first.
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    ak.enum AS account_kind_enum,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    tak.enum AS to_account_kind_enum,
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
    ta.deleted_at AS to_account_deleted_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.id AS goal_id,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    gc.in_grams AS goal_currency_in_grams,
    CAST(COALESCE((
        SELECT sm.body FROM sms_messages AS sm WHERE sm.transaction_id = t.id LIMIT 1
    ), '') AS TEXT) AS sms_body,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN account_kinds AS ak ON a.kind_id = ak.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN account_kinds AS tak ON ta.kind_id = tak.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
-- A goal's transactions are the ones in or out of its accounts, and the
-- payments of the loan it was bought with.
LEFT JOIN goals AS g ON g.id IN (a.goal_id, ta.goal_id) OR g.id = t.loan_goal_id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE t.reimburses_id IN (SELECT value FROM json_each(sqlc.arg(ids)))
ORDER BY t.scheduled_to DESC, t.created_at DESC;

-- name: GetTransferPairsOfMany :many
-- The incoming legs paired with the outgoing ones of ids (a JSON array).
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    ak.enum AS account_kind_enum,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    tak.enum AS to_account_kind_enum,
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
    ta.deleted_at AS to_account_deleted_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.id AS goal_id,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    gc.in_grams AS goal_currency_in_grams,
    CAST(COALESCE((
        SELECT sm.body FROM sms_messages AS sm WHERE sm.transaction_id = t.id LIMIT 1
    ), '') AS TEXT) AS sms_body,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN account_kinds AS ak ON a.kind_id = ak.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN account_kinds AS tak ON ta.kind_id = tak.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
-- A goal's transactions are the ones in or out of its accounts, and the
-- payments of the loan it was bought with.
LEFT JOIN goals AS g ON g.id IN (a.goal_id, ta.goal_id) OR g.id = t.loan_goal_id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE t.transfer_pair_id IN (SELECT value FROM json_each(sqlc.arg(ids)));

-- name: GetTransferPairCandidates :many
-- Unpaired transactions of kind (1 expense, 2 income) out of account_id,
-- fulfilled between since and until, that could be the other leg of a
-- transfer: not a goal's nor a deleted account's, not reimbursing nor
-- reimbursed. With sms_only (auto pairing), only SMS ones and no merchant.
SELECT
    sqlc.embed(t),
    sqlc.embed(a),
    sqlc.embed(c),
    ak.enum AS account_kind_enum,
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    CAST(COALESCE(cp.is_merchant, FALSE) AS BOOLEAN) AS counterparty_is_merchant,
    ta.id AS to_account_id,
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    tak.enum AS to_account_kind_enum,
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
    ta.deleted_at AS to_account_deleted_at,
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,
    cat.id AS category_id,
    cat.name AS category_name,
    cat.color AS category_color,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.created_at AS category_created_at,
    g.id AS goal_id,
    g.name AS goal_name,
    g.price AS goal_price,
    g.currency_id AS goal_currency_id,
    g.fulfilled_at AS goal_fulfilled_at,
    gc.string AS goal_currency_string,
    gc.symbol AS goal_currency_symbol,
    gc.in_grams AS goal_currency_in_grams,
    CAST(COALESCE((
        SELECT sm.body FROM sms_messages AS sm WHERE sm.transaction_id = t.id LIMIT 1
    ), '') AS TEXT) AS sms_body,
    CAST((
        SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
    ) AS FLOAT) AS reimbursed
FROM transactions AS t
LEFT JOIN accounts AS a ON t.account_id = a.id
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN account_kinds AS ak ON a.kind_id = ak.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
LEFT JOIN account_kinds AS tak ON ta.kind_id = tak.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
-- A goal's transactions are the ones in or out of its accounts, and the
-- payments of the loan it was bought with.
LEFT JOIN goals AS g ON g.id IN (a.goal_id, ta.goal_id) OR g.id = t.loan_goal_id
LEFT JOIN currencies AS gc ON g.currency_id = gc.id
WHERE
    t.kind = sqlc.arg(kind)
    AND t.account_id != sqlc.arg(account_id)
    AND t.id != sqlc.arg(id)
    AND t.transfer_pair_id IS NULL
    AND t.reimburses_id IS NULL
    AND ak.enum != 6 -- models.AccKindGoal
    AND a.deleted_at IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND JULIANDAY(t.fulfilled_at) BETWEEN JULIANDAY(sqlc.arg(since)) AND JULIANDAY(sqlc.arg(until))
    AND NOT EXISTS (SELECT 1 FROM transactions AS rt WHERE rt.reimburses_id = t.id)
    AND (CAST(sqlc.arg(sms_only) AS BOOLEAN) = FALSE OR (
        cp.is_merchant IS NOT TRUE
        AND EXISTS (SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id)
    ))
ORDER BY t.fulfilled_at;

-- name: GetUnpairedSMSTransactions :many
-- Ids of the SMS expenses and incomes that could still be a transfer leg,
-- oldest first.
SELECT t.id
FROM transactions AS t
LEFT JOIN counterparties AS cp ON t.counterparty_id = cp.id
WHERE
    t.kind IN (1, 2)
    AND t.transfer_pair_id IS NULL
    AND t.reimburses_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND cp.is_merchant IS NOT TRUE
    AND EXISTS (SELECT 1 FROM sms_messages AS sm WHERE sm.transaction_id = t.id)
ORDER BY t.fulfilled_at;

-- name: SetTransferPair :exec
-- Pairs a and b, each pointing at the other.
UPDATE transactions
SET
    transfer_pair_id = IIF(id = sqlc.arg(a), CAST(sqlc.arg(b) AS TEXT), sqlc.arg(a))
WHERE id IN (sqlc.arg(a), sqlc.arg(b));

-- name: ClearTransferPair :exec
-- Unpairs id and its other leg.
UPDATE transactions
SET
    transfer_pair_id = NULL
WHERE id = sqlc.arg(id) OR transfer_pair_id = sqlc.arg(id);

-- name: GetDailyFlow :many
-- Fulfilled income (kind 2) and spending (kind 1 net of its reimbursements,
-- minus refunds, kind 5) per day and account currency since from_date,
-- following the analysis rule. Days are scheduled_to's, as the transactions
-- list groups them; currencies are merged in Go using live fx rates.
SELECT
    CAST(SUBSTR(t.scheduled_to, 1, 10) AS TEXT) AS day,
    sqlc.embed(cur),
    CAST(TOTAL(IIF(t.kind = 2, t.amount, 0)) AS FLOAT) AS income,
    CAST(TOTAL(CASE t.kind
        WHEN 1 THEN t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        )
        WHEN 5 THEN -t.amount
        ELSE 0
    END) AS FLOAT) AS expenses
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 2, 5)
    -- Reimbursements are netted from their expense.
    AND t.reimburses_id IS NULL
    -- Transfer pairs move money between own accounts.
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
GROUP BY day, cur.id
ORDER BY day;

-- name: GetCategorySpending :many
-- Fulfilled spending (expenses net of their reimbursements, minus refunds)
-- per category (empty for none) and account currency between from_date and
-- to_date (exclusive).
SELECT
    CAST(IFNULL(t.category_id, '') AS TEXT) AS category_id,
    sqlc.embed(cur),
    CAST(TOTAL(IIF(
        t.kind = 1,
        t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        ),
        -t.amount
    )) AS FLOAT) AS total
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 5)
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date)
GROUP BY t.category_id, cur.id;

-- name: GetDailyCategorySpending :many
-- GetCategorySpending per day (scheduled_to's) too, for the budget summary's
-- burn-up chart.
SELECT
    CAST(SUBSTR(t.scheduled_to, 1, 10) AS TEXT) AS day,
    CAST(IFNULL(t.category_id, '') AS TEXT) AS category_id,
    sqlc.embed(cur),
    CAST(TOTAL(IIF(
        t.kind = 1,
        t.amount - (
            SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id
        ),
        -t.amount
    )) AS FLOAT) AS total
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind IN (1, 5)
    AND t.reimburses_id IS NULL
    AND t.transfer_pair_id IS NULL
    AND t.fulfilled_at IS NOT NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date)
GROUP BY day, t.category_id, cur.id
ORDER BY day;

-- name: GetScheduledCategorySpending :many
-- The unfulfilled expenses per day (scheduled_to's, overdue ones counted on
-- from_date), category and currency before to_date: what's already planned
-- out of the budgets, for the accounts summary's forecast.
SELECT
    CAST(MAX(SUBSTR(t.scheduled_to, 1, 10), SUBSTR(sqlc.arg(from_date), 1, 10)) AS TEXT) AS day,
    CAST(IFNULL(t.category_id, '') AS TEXT) AS category_id,
    sqlc.embed(cur),
    CAST(TOTAL(t.amount) AS FLOAT) AS total
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind = 1
    AND t.fulfilled_at IS NULL
    AND t.category_id IS NOT NULL
    AND t.scheduled_to < sqlc.arg(to_date)
GROUP BY day, t.category_id, cur.id
ORDER BY day;

-- name: GetScheduledIncomes :many
-- The unfulfilled incomes scheduled from from_date to to_date (exclusive),
-- for the accounts summary's forecast to tell a salary already scheduled.
SELECT
    t.amount,
    sqlc.embed(cur)
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
WHERE
    t.kind = 2
    AND t.fulfilled_at IS NULL
    AND t.scheduled_to >= sqlc.arg(from_date)
    AND t.scheduled_to < sqlc.arg(to_date);

-- name: GetFirstFulfilledTransactionDate :one
SELECT CAST(IFNULL(MIN(SUBSTR(scheduled_to, 1, 10)), '') AS TEXT)
FROM transactions
WHERE fulfilled_at IS NOT NULL;

-- name: GetUnbudgetedScheduledOutflows :many
-- The unfulfilled money scheduled to leave the balance before to_date (the
-- overdue included) that no budget covers, per currency, for the
-- dashboard's safe to spend: expenses of a category without a budget (or
-- none), but a goal's loan payments (its plan asks them), and transfers /
-- debts to an account out of the balance (a gam'yaa, a goal, lent money).
SELECT
    sqlc.embed(cur),
    CAST(TOTAL(t.amount) AS FLOAT) AS total
FROM transactions AS t
JOIN accounts AS a ON t.account_id = a.id
JOIN currencies AS cur ON a.currency_id = cur.id
LEFT JOIN categories AS cat ON t.category_id = cat.id
LEFT JOIN accounts AS ta ON t.to_account_id = ta.id
WHERE
    t.fulfilled_at IS NULL
    AND t.scheduled_to < sqlc.arg(to_date)
    AND a.is_archived IS FALSE
    AND a.exclude_from_balance IS FALSE
    AND a.deleted_at IS NULL
    AND (
        (
            t.kind = 1
            AND t.loan_goal_id IS NULL
            AND (cat.id IS NULL OR cat.budget_currency_id IS NULL OR IFNULL(cat.budget_amount, 0) <= 0)
        )
        OR (
            t.kind IN (3, 4)
            AND (
                ta.id IS NULL
                OR ta.is_archived IS TRUE
                OR ta.exclude_from_balance IS TRUE
                OR ta.deleted_at IS NOT NULL
            )
        )
    )
GROUP BY cur.id;
