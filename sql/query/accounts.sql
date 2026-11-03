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
        JOIN goals AS gl ON gl.account_id IN (t.account_id, t.to_account_id)
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND gl.account_id != a.id
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
        JOIN goals AS gl ON gl.account_id IN (t.account_id, t.to_account_id)
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND gl.account_id != a.id
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
        JOIN goals AS gl ON gl.account_id IN (t.account_id, t.to_account_id)
        WHERE
            t.kind = 3
            AND t.fulfilled_at IS NOT NULL
            AND gl.fulfilled_at IS NULL
            AND a.id IN (t.account_id, t.to_account_id)
            AND gl.account_id != a.id
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
    sms_refs
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
    sqlc.arg(sms_refs)
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
    sms_refs = sqlc.arg(sms_refs)
WHERE id = sqlc.arg(id);

-- name: DeleteAccount :exec
-- Soft deletes: the account stays for the history of its transactions.
UPDATE accounts
SET deleted_at = DATETIME('now')
WHERE id = sqlc.arg(id);

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
    AND NOT EXISTS (SELECT 1 FROM goals AS gl WHERE gl.account_id = accounts.id);

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
