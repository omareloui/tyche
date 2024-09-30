-- name: CreateFrequentTx :exec
INSERT INTO frequent_txs (
    id,
    frequency_by,
    frequency,
    account_id,
    amount,
    kind_id,
    description,
    stops_on,
    starts_on,
    last_transaction_created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateFrequentTxTransaction :exec
INSERT INTO frequent_txs_transactions (
    transaction_id,
    frequency_id
)
VALUES (?, ?);

-- name: GetFrequentTxs :many
SELECT
    ft.id,
    ft.account_id,
    ft.amount,
    ft.kind_id,
    ft.description,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.starts_on,
    ft.stops_on,
    ft.last_transaction_created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.description AS transaction_description,
    t.fulfilled_on AS transaction_fulfilled_on,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.currency_id AS transaction_currency_id,
    tc.enum AS transaction_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tk.kind AS transaction_kind,
    tk.increase_balance AS transaction_increase_balance
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = c.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id;


-- name: GetFrequentTx :many
SELECT
    ft.id,
    ft.account_id,
    ft.amount,
    ft.kind_id,
    ft.description,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.starts_on,
    ft.stops_on,
    ft.last_transaction_created_at,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.description AS transaction_description,
    t.fulfilled_on AS transaction_fulfilled_on,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.currency_id AS transaction_currency_id,
    tc.enum AS transaction_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tk.kind AS transaction_kind,
    tk.increase_balance AS transaction_increase_balance
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = c.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
WHERE ft.id = ?;

-- name: UpdateFrequentTx :exec
UPDATE frequent_txs
SET
frequency_by = ?,
frequency = ?,
account_id = ?,
amount = ?,
kind_id = ?,
description = ?,
stops_on = ?,
last_transaction_created_at = ?
WHERE id = ?;


-- name: UpdateFrequentTxLastTransactionCreatedAt :exec
UPDATE frequent_txs
SET
last_transaction_created_at = ?
WHERE id = ?;


-- name: DeleteFrequentTx :exec
DELETE FROM frequent_txs
WHERE id = ?;

-- name: DeleteScheduledTransactionsOfFrequentTx :exec
DELETE FROM transactions
WHERE ROWID IN (
    SELECT tx.ROWID FROM transactions AS tx
    LEFT JOIN frequent_txs_transactions AS ftxt ON ftxt.transaction_id = tx.id 
    WHERE
        ftxt.frequency_id = ? AND
        tx.fulfilled_on IS NULL
);
