-- name: CreateFrequentTx :exec
INSERT INTO frequent_txs (
    id,
    frequency_by,
    frequency,
    account_id,
    amount,
    kind_id,
    category_id,
    counterparty_id,
    description,
    to_account_id,
    to_amount,
    starts_at,
    stops_at,
    last_transaction_scheduled_to
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(frequency_by),
    sqlc.arg(frequency),
    sqlc.arg(account_id),
    ROUND(sqlc.arg(amount), 2),
    sqlc.arg(kind_id),
    sqlc.arg(category_id),
    sqlc.arg(counterparty_id),
    sqlc.arg(description),
    sqlc.arg(to_account_id),
    IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    sqlc.arg(starts_at),
    sqlc.arg(stops_at),
    sqlc.arg(last_transaction_scheduled_to)
);

-- name: CreateFrequentTxTransaction :exec
INSERT INTO frequent_txs_transactions (
    transaction_id,
    frequency_id
)
VALUES (
    sqlc.arg(transaction_id),
    sqlc.arg(frequency_id)
);

-- name: GetFrequentTxs :many
SELECT
    ft.id,
    ft.account_id,
    ft.amount,
    ft.kind_id,
    ft.category_id,
    ft.counterparty_id,
    ft.description,
    ft.to_account_id,
    ft.to_amount,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.starts_at,
    ft.stops_at,
    ft.last_transaction_scheduled_to,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    cp.name AS counterparty_name,
    ftta.name AS to_account_name,
    ftta.currency_id AS to_currency_id,
    fttc.enum AS to_currency_enum,
    fttc.egp_fx_rate AS to_egp_fx_rate,
    k.enum AS kind_enum,
    k.increase_balance,
    cat.name AS category_name,
    cat.budget_in_egp AS category_budget_in_egp,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.currency_id AS transaction_currency_id,
    tc.enum AS transaction_currency_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tcp.name AS transaction_counterparty_name,
    tta.currency_id AS transaction_to_currency_id,
    ttc.enum AS transaction_to_currency_enum,
    ttc.egp_fx_rate AS transaction_to_egp_fx_rate,
    tk.enum AS transaction_kind_enum,
    tk.increase_balance AS transaction_increase_balance,
    tcat.name AS transaction_category_name,
    tcat.budget_in_egp AS transaction_category_budget_in_egp
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN accounts AS tta ON t.to_account_id = tta.id
LEFT JOIN currencies AS ttc ON tta.currency_id = ttc.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
LEFT JOIN categories AS tcat ON t.category_id = tcat.id
LEFT JOIN counterparties AS tcp ON t.counterparty_id = tcp.id;


-- name: GetFrequentTx :many
SELECT
    ft.id,
    ft.account_id,
    ft.amount,
    ft.kind_id,
    ft.category_id,
    ft.counterparty_id,
    ft.description,
    ft.to_account_id,
    ft.to_amount,
    ft.created_at,
    ft.frequency_by,
    ft.frequency,
    ft.starts_at,
    ft.stops_at,
    ft.last_transaction_scheduled_to,
    a.name AS account_name,
    a.currency_id,
    c.enum AS currency_enum,
    c.egp_fx_rate,
    cp.name AS counterparty_name,
    ftta.name AS to_account_name,
    ftta.currency_id AS to_currency_id,
    fttc.enum AS to_currency_enum,
    fttc.egp_fx_rate AS to_egp_fx_rate,
    k.enum AS kind_enum,
    k.increase_balance,
    cat.name AS category_name,
    cat.budget_in_egp AS category_budget_in_egp,
    t.id AS transaction_id,
    t.account_id AS transaction_account_id,
    t.amount AS transaction_amount,
    t.kind_id AS transaction_kind_id,
    t.counterparty_id AS transaction_counterparty_id,
    t.description AS transaction_description,
    t.to_account_id AS transaction_to_account_id,
    t.to_amount AS transaction_to_amount,
    t.fulfilled_at AS transaction_fulfilled_at,
    t.scheduled_to AS transaction_scheduled_to,
    t.created_at AS transaction_created_at,
    ta.currency_id AS transaction_currency_id,
    tc.enum AS transaction_currency_enum,
    tc.egp_fx_rate AS transaction_egp_fx_rate,
    tcp.name AS transaction_counterparty_name,
    tta.currency_id AS transaction_to_currency_id,
    ttc.enum AS transaction_to_currency_enum,
    ttc.egp_fx_rate AS transaction_to_egp_fx_rate,
    tk.enum AS transaction_kind_enum,
    tk.increase_balance AS transaction_increase_balance,
    tcat.name AS transaction_category_name,
    tcat.budget_in_egp AS transaction_category_budget_in_egp
FROM frequent_txs AS ft
LEFT JOIN frequent_txs_transactions AS ftt ON ftt.frequency_id = ft.id
LEFT JOIN counterparties AS cp ON ft.counterparty_id = cp.id
LEFT JOIN accounts AS a ON ft.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ftta ON ft.to_account_id = a.id
LEFT JOIN currencies AS fttc ON ftta.currency_id = c.id
LEFT JOIN transaction_kinds AS k ON ft.kind_id = k.id
LEFT JOIN categories AS cat ON ft.category_id = cat.id
LEFT JOIN transactions AS t ON t.id = ftt.transaction_id
LEFT JOIN accounts AS ta ON t.account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN accounts AS tta ON t.to_account_id = tta.id
LEFT JOIN currencies AS ttc ON tta.currency_id = ttc.id
LEFT JOIN transaction_kinds AS tk ON t.kind_id = k.id
LEFT JOIN categories AS tcat ON t.category_id = tcat.id
LEFT JOIN counterparties AS tcp ON t.counterparty_id = tcp.id
WHERE ft.id = sqlc.arg(id);

-- name: UpdateFrequentTx :exec
UPDATE frequent_txs
SET
    frequency_by = sqlc.arg(frequency_by),
    frequency = sqlc.arg(frequency),
    account_id = sqlc.arg(account_id),
    amount = ROUND(sqlc.arg(amount), 2),
    kind_id = sqlc.arg(kind_id),
    category_id = sqlc.arg(category_id),
    counterparty_id = sqlc.arg(counterparty_id),
    description = sqlc.arg(description),
    to_account_id = sqlc.arg(to_account_id),
    to_amount = IIF(sqlc.narg(to_amount) > 0.0, ROUND(sqlc.narg(to_amount), 2), NULL),
    stops_at = sqlc.arg(stops_at),
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: UpdateFrequentTxLastTransactionCreatedAt :exec
UPDATE frequent_txs
SET
    last_transaction_scheduled_to = sqlc.arg(last_transaction_scheduled_to)
WHERE id = sqlc.arg(id);


-- name: DeleteFrequentTx :exec
DELETE FROM frequent_txs
WHERE id = sqlc.arg(id);

-- name: DeleteScheduledTransactionsOfFrequentTx :exec
DELETE FROM transactions
WHERE rowid IN (
    SELECT tx.rowid FROM transactions AS tx
    LEFT JOIN frequent_txs_transactions AS ftxt ON tx.id = ftxt.transaction_id
    WHERE
        ftxt.frequency_id = sqlc.arg(frequency_id)
        AND tx.fulfilled_at IS NULL
);
