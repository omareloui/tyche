-- name: CreateFrequentTx :exec
INSERT INTO frequent_txs (
    id,
    frequency_by,
    frequency,
    amount,
    currency_id,
    kind_id,
    description,
    stops_on,
    created_at,
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
    ft.amount,
    ft.currency_id,
    ft.kind_id,
    ft.description,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.stops_on,
    c.enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance,
    t.id AS transaction_id,
    t.amount AS transaction_amount,
    t.currency_id AS transaction_currency_id,
    t.kind_id AS transaction_kind_id,
    t.description AS transaction_description,
    t.fulfilled_on AS transaction_fulfilled_on,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    tc.enum AS transaction_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tk.kind AS transaction_kind,
    tk.increase_balance AS transaction_increase_balance
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN currencies AS c ON ft.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN currencies AS tc ON t.currency_id = c.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id;


-- name: GetFrequentTx :many
SELECT
    ft.id,
    ft.amount,
    ft.currency_id,
    ft.kind_id,
    ft.description,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.stops_on,
    c.enum,
    c.egp_fx_rate,
    k.kind,
    k.increase_balance,
    t.id AS transaction_id,
    t.amount AS transaction_amount,
    t.currency_id AS transaction_currency_id,
    t.kind_id AS transaction_kind_id,
    t.description AS transaction_description,
    t.fulfilled_on AS transaction_fulfilled_on,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    tc.enum AS transaction_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tk.kind AS transaction_kind,
    tk.increase_balance AS transaction_increase_balance
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN currencies AS c ON ft.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN currencies AS tc ON t.currency_id = c.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
WHERE ft.id = ?;


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
