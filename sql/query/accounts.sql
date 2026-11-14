-- name: GetAccounts :many
SELECT
    sqlc.embed(a),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default,
    -- Money moved from the account to goals still in progress (net of what
    -- came back), out of its balance though still in the bank.
    CAST((
        SELECT TOTAL(IIF(t.account_id = a.id, t.amount, -COALESCE(t.to_amount, t.amount)))
        FROM transactions AS t
        JOIN accounts AS ga ON ga.goal_id IS NOT NULL AND ga.id IN (t.account_id, t.to_account_id)
        JOIN goals AS gl ON gl.id = ga.goal_id
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND ga.id != a.id
    ) AS FLOAT) AS set_aside
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN user AS u ON u.default_account_id = a.id
-- Live accounts only: the deleted ones and the goals' are hidden.
WHERE a.deleted_at IS NULL AND k.enum != 6 -- models.AccKindGoal
ORDER BY a.created_at DESC;

-- name: GetActiveAccounts :many
SELECT
    sqlc.embed(a),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default,
    -- Money moved from the account to goals still in progress (net of what
    -- came back), out of its balance though still in the bank.
    CAST((
        SELECT TOTAL(IIF(t.account_id = a.id, t.amount, -COALESCE(t.to_amount, t.amount)))
        FROM transactions AS t
        JOIN accounts AS ga ON ga.goal_id IS NOT NULL AND ga.id IN (t.account_id, t.to_account_id)
        JOIN goals AS gl ON gl.id = ga.goal_id
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND ga.id != a.id
    ) AS FLOAT) AS set_aside
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN user AS u ON u.default_account_id = a.id
WHERE a.is_archived IS FALSE AND a.deleted_at IS NULL AND k.enum != 6 -- models.AccKindGoal
ORDER BY a.created_at DESC;

-- name: GetAccount :one
SELECT
    sqlc.embed(a),
    sqlc.embed(k),
    cp.name AS counterparty_name,
    cp.icon AS counterparty_icon,
    cp.color AS counterparty_color,
    sqlc.embed(c),
    (u.id IS NOT NULL) AS is_default,
    -- Money moved from the account to goals still in progress (net of what
    -- came back), out of its balance though still in the bank.
    CAST((
        SELECT TOTAL(IIF(t.account_id = a.id, t.amount, -COALESCE(t.to_amount, t.amount)))
        FROM transactions AS t
        JOIN accounts AS ga ON ga.goal_id IS NOT NULL AND ga.id IN (t.account_id, t.to_account_id)
        JOIN goals AS gl ON gl.id = ga.goal_id
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND ga.id != a.id
    ) AS FLOAT) AS set_aside
FROM accounts AS a
LEFT JOIN account_kinds AS k ON a.kind_id = k.id
LEFT JOIN counterparties AS cp ON a.counterparty_id = cp.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN user AS u ON u.default_account_id = a.id
WHERE a.id = sqlc.arg(id);

-- name: CreateAccount :exec
INSERT INTO accounts (
    id,
    name,
    kind_id,
    amount,
    currency_id,
    counterparty_id,
    card_number,
    card_limit,
    name_on_card,
    iban,
    note,
    exclude_from_balance,
    is_archived,
    expiration_date,
    sms_refs,
    statement_day,
    due_days,
    due_day
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(kind_id),
    sqlc.arg(amount),
    sqlc.arg(currency_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(card_number),
    sqlc.arg(card_limit),
    sqlc.arg(name_on_card),
    sqlc.arg(iban),
    sqlc.arg(note),
    sqlc.arg(exclude_from_balance),
    sqlc.arg(is_archived),
    sqlc.arg(expiration_date),
    sqlc.arg(sms_refs),
    sqlc.narg(statement_day),
    sqlc.narg(due_days),
    sqlc.narg(due_day)
);

-- name: UpdateAccount :exec
UPDATE accounts
SET
    name = sqlc.arg(name),
    kind_id = sqlc.arg(kind_id),
    currency_id = sqlc.arg(currency_id),
    counterparty_id = sqlc.arg(counterparty_id),
    card_number = sqlc.arg(card_number),
    card_limit = sqlc.arg(card_limit),
    name_on_card = sqlc.arg(name_on_card),
    iban = sqlc.arg(iban),
    note = sqlc.arg(note),
    is_archived = sqlc.arg(is_archived),
    exclude_from_balance = sqlc.arg(exclude_from_balance),
    expiration_date = sqlc.arg(expiration_date),
    sms_refs = sqlc.arg(sms_refs),
    statement_day = sqlc.narg(statement_day),
    due_days = sqlc.narg(due_days),
    due_day = sqlc.narg(due_day)
WHERE id = sqlc.arg(id);

-- name: DeleteAccount :exec
-- Soft deletes: the account stays for the history of its transactions.
UPDATE accounts
SET deleted_at = DATETIME('now')
WHERE id = sqlc.arg(id);

-- name: GetCardActivitySince :one
-- What a credit card's transactions did to its balance: net is the signed
-- total of the ones fulfilled since its statement closed, paid_in only what
-- they added (payments, refunds), and scheduled the signed total of the
-- unfulfilled ones due before until (the end of the next statement's day).
SELECT
    CAST(TOTAL(IIF(e.fulfilled, e.effect, 0)) AS FLOAT) AS net,
    CAST(TOTAL(IIF(e.fulfilled, MAX(e.effect, 0), 0)) AS FLOAT) AS paid_in,
    CAST(TOTAL(IIF(e.fulfilled, 0, e.effect)) AS FLOAT) AS scheduled
FROM (
    SELECT
        t.fulfilled_at IS NOT NULL AS fulfilled,
        CASE
            WHEN t.kind = 3 AND t.to_account_id = sqlc.arg(account_id) THEN COALESCE(t.to_amount, t.amount)
            WHEN t.kind IN (2, 5, 6) THEN t.amount -- income, refund, adjustment
            ELSE -t.amount
        END AS effect
    FROM transactions AS t
    WHERE
        (t.account_id = sqlc.arg(account_id) OR t.to_account_id = sqlc.arg(account_id))
        AND (
            (t.fulfilled_at IS NOT NULL AND JULIANDAY(t.fulfilled_at) >= JULIANDAY(sqlc.arg(since)))
            OR (t.fulfilled_at IS NULL AND JULIANDAY(t.scheduled_to) < JULIANDAY(sqlc.arg(until)))
        )
) AS e;

-- name: AddToAccount :exec
UPDATE accounts
SET
    amount = ROUND(amount + sqlc.arg(amount), 6)
WHERE id = sqlc.arg(id);

-- name: RemoveFromAccount :exec
UPDATE accounts
SET
    amount = ROUND(amount - sqlc.arg(amount), 6)
WHERE id = sqlc.arg(id);

-- name: GetAccountAmount :one
SELECT amount FROM accounts
WHERE id = sqlc.arg(id);

-- name: SetAccountAmount :exec
UPDATE accounts
SET
    amount = ROUND(sqlc.arg(amount), 6)
WHERE id = sqlc.arg(id);

-- name: SetCurrencyOfUnusedAccounts :exec
UPDATE accounts
SET currency_id = sqlc.arg(currency_id)
WHERE
    accounts.currency_id = sqlc.arg(from_currency_id)
    AND NOT EXISTS (SELECT 1 FROM transactions AS t WHERE t.account_id = accounts.id OR t.to_account_id = accounts.id)
    AND NOT EXISTS (SELECT 1 FROM recurrent_txs AS rt WHERE rt.account_id = accounts.id OR rt.to_account_id = accounts.id)
    AND NOT EXISTS (SELECT 1 FROM gamyaas AS g WHERE g.account_id = accounts.id)
    AND NOT EXISTS (SELECT 1 FROM goals AS gl WHERE gl.account_id = accounts.id)
    AND accounts.goal_id IS NULL;

-- name: GetBalances :many
SELECT
    a.currency_id,
    sqlc.embed(c),
    TOTAL(a.amount) AS balance
FROM accounts AS a
LEFT JOIN currencies AS c ON a.currency_id = c.id
WHERE
    a.is_archived IS FALSE
    AND a.exclude_from_balance IS FALSE
    AND a.deleted_at IS NULL
GROUP BY a.currency_id ORDER BY c.string;

-- name: DeleteRecurrentTxsOfAccount :exec
-- Stops the recurrent transactions of a deleted account.
DELETE FROM recurrent_txs
WHERE account_id = sqlc.arg(id) OR to_account_id = sqlc.arg(id);

-- name: DeleteScheduledTxsOfAccount :exec
-- Unfulfilled ones never touched the balances, so they just go.
DELETE FROM transactions
WHERE (account_id = sqlc.arg(id) OR to_account_id = sqlc.arg(id)) AND fulfilled_at IS NULL;

-- name: ClearDefaultAccount :exec
UPDATE user
SET default_account_id = NULL
WHERE default_account_id = sqlc.arg(id);

-- name: GetAccountDailyNet :many
-- What the account's fulfilled transactions moved in and out of it per day
-- (of scheduled_to, the transaction's date as the lists show it, not when it
-- was marked fulfilled, e.g. past ones backfilled later) since since: the
-- raw money movement, transfers and debts included, not an analysis (so not
-- following the analysis rule), to walk its balance back from the current
-- one.
SELECT
    CAST(e.day AS TEXT) AS day,
    CAST(TOTAL(MAX(e.effect, 0)) AS FLOAT) AS money_in,
    CAST(TOTAL(MAX(-e.effect, 0)) AS FLOAT) AS money_out
FROM (
    SELECT
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        CASE
            WHEN t.kind IN (3, 4) AND t.to_account_id = sqlc.arg(account_id) THEN COALESCE(t.to_amount, t.amount)
            WHEN t.kind IN (2, 5, 6) THEN t.amount -- income, refund, adjustment
            ELSE -t.amount
        END AS effect
    FROM transactions AS t
    WHERE
        (t.account_id = sqlc.arg(account_id) OR t.to_account_id = sqlc.arg(account_id))
        AND t.fulfilled_at IS NOT NULL
        AND JULIANDAY(t.scheduled_to) >= JULIANDAY(sqlc.arg(since))
) AS e
GROUP BY e.day
ORDER BY e.day;

-- name: GetAccountScheduledDailyNet :many
-- What the account's unfulfilled transactions will move in and out of it
-- per day (of scheduled_to) before until, overdue ones included, like
-- GetAccountDailyNet: to project its balance ahead.
SELECT
    CAST(e.day AS TEXT) AS day,
    CAST(TOTAL(MAX(e.effect, 0)) AS FLOAT) AS money_in,
    CAST(TOTAL(MAX(-e.effect, 0)) AS FLOAT) AS money_out
FROM (
    SELECT
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        CASE
            WHEN t.kind IN (3, 4) AND t.to_account_id = sqlc.arg(account_id) THEN COALESCE(t.to_amount, t.amount)
            WHEN t.kind IN (2, 5, 6) THEN t.amount -- income, refund, adjustment
            ELSE -t.amount
        END AS effect
    FROM transactions AS t
    WHERE
        (t.account_id = sqlc.arg(account_id) OR t.to_account_id = sqlc.arg(account_id))
        AND t.fulfilled_at IS NULL
        AND JULIANDAY(t.scheduled_to) < JULIANDAY(sqlc.arg(until))
) AS e
GROUP BY e.day
ORDER BY e.day;

-- name: GetAccountsDailyNet :many
-- GetAccountDailyNet of every account at once, for the accounts summary's
-- net worth chart: each fulfilled transaction moves its account, and a
-- transfer or debt its to account too.
SELECT
    CAST(e.account_id AS TEXT) AS account_id,
    CAST(e.day AS TEXT) AS day,
    CAST(TOTAL(MAX(e.effect, 0)) AS FLOAT) AS money_in,
    CAST(TOTAL(MAX(-e.effect, 0)) AS FLOAT) AS money_out
FROM (
    SELECT
        t.account_id AS account_id,
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        IIF(t.kind IN (2, 5, 6), t.amount, -t.amount) AS effect
    FROM transactions AS t
    WHERE
        t.fulfilled_at IS NOT NULL
        AND JULIANDAY(t.scheduled_to) >= JULIANDAY(sqlc.arg(since))
    UNION ALL
    SELECT
        t.to_account_id AS account_id,
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        COALESCE(t.to_amount, t.amount) AS effect
    FROM transactions AS t
    WHERE
        t.kind IN (3, 4)
        AND t.to_account_id IS NOT NULL
        AND t.fulfilled_at IS NOT NULL
        AND JULIANDAY(t.scheduled_to) >= JULIANDAY(sqlc.arg(since))
) AS e
GROUP BY e.account_id, e.day
ORDER BY e.day;

-- name: GetAccountsScheduledDailyNet :many
-- GetAccountScheduledDailyNet of every account at once, like
-- GetAccountsDailyNet: the accounts summary's forecast.
SELECT
    CAST(e.account_id AS TEXT) AS account_id,
    CAST(e.day AS TEXT) AS day,
    CAST(TOTAL(MAX(e.effect, 0)) AS FLOAT) AS money_in,
    CAST(TOTAL(MAX(-e.effect, 0)) AS FLOAT) AS money_out
FROM (
    SELECT
        t.account_id AS account_id,
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        IIF(t.kind IN (2, 5, 6), t.amount, -t.amount) AS effect
    FROM transactions AS t
    WHERE
        t.fulfilled_at IS NULL
        AND JULIANDAY(t.scheduled_to) < JULIANDAY(sqlc.arg(until))
    UNION ALL
    SELECT
        t.to_account_id AS account_id,
        SUBSTR(t.scheduled_to, 1, 10) AS day,
        COALESCE(t.to_amount, t.amount) AS effect
    FROM transactions AS t
    WHERE
        t.kind IN (3, 4)
        AND t.to_account_id IS NOT NULL
        AND t.fulfilled_at IS NULL
        AND JULIANDAY(t.scheduled_to) < JULIANDAY(sqlc.arg(until))
) AS e
GROUP BY e.account_id, e.day
ORDER BY e.day;

-- name: GetAccountTxStats :one
-- How many fulfilled transactions touched the account, and the dates of the
-- first and last.
SELECT
    COUNT(*) AS count,
    CAST(IFNULL(MIN(t.scheduled_to), '') AS TEXT) AS first_at,
    CAST(IFNULL(MAX(t.scheduled_to), '') AS TEXT) AS last_at
FROM transactions AS t
WHERE
    (t.account_id = sqlc.arg(account_id) OR t.to_account_id = sqlc.arg(account_id))
    AND t.fulfilled_at IS NOT NULL;

-- name: SetAccountNote :exec
UPDATE accounts
SET note = sqlc.narg(note)
WHERE id = sqlc.arg(id);
